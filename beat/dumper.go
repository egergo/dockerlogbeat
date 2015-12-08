package dockerlogbeat

import (
	"os"
	"strings"
	"sync"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/fsouza/go-dockerclient"
)

type Dumper struct {
	DockerClient *docker.Client
	Registry     *Registry
	Target       chan *DockerLogEvent

	containers map[string]bool
	mutex      sync.Mutex
	events     chan *docker.APIEvents
}

func NewDumper(client *docker.Client, registry *Registry, target chan *DockerLogEvent) *Dumper {
	dumper := Dumper{
		DockerClient: client,
		Registry:     registry,
		Target:       target,

		containers: make(map[string]bool),
		events:     make(chan *docker.APIEvents),
	}
	return &dumper
}

func (dumper *Dumper) StartEventChannel() error {
	if err := dumper.DockerClient.AddEventListener(dumper.events); err != nil {
		return err
	}

	logp.Info("Listening on Docker event channel")
	go func() {
		defer logp.Info("Stopped listening on Docker event channel")
		for {
			event, alive := <-dumper.events
			if !alive {
				return
			}
			if event.Status == "start" || event.Status == "restart" {
				if err := dumper.ScanContainers(); err != nil {
					logp.Err("Cannot scan containers: %s", err)
					panic(err)
				}
			}
		}
	}()

	return nil
}

func (dumper *Dumper) StopEventChannel() error {
	return dumper.DockerClient.RemoveEventListener(dumper.events)
}

func (dumper *Dumper) ScanContainers() error {
	dumper.mutex.Lock()
	defer dumper.mutex.Unlock()

	logp.Info("Scanning Docker containers")

	containers, err := dumper.DockerClient.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return err
	}
	for _, c := range containers {
		if dumper.containers[c.ID] {
			continue
		}

		container, err := dumper.DockerClient.InspectContainer(c.ID)
		if err != nil {
			return err
		}

		if container.Config.Tty {
			continue
		}

		if SliceContains(container.Config.Env, "LOGSPOUT=ignore") {
			continue
		}

		if SliceContains(container.Config.Env, "DLB=ignore") {
			continue
		}

		var tag string
		for _, envvar := range container.Config.Env {
			if strings.HasPrefix(envvar, "DLB_TAG=") {
				tag = envvar[8:]
				break
			}
		}

		containerInfo := ContainerInfo{
			ContainerID:   container.ID,
			ContainerName: container.Name[1:],
			ImageID:       container.Image,
			ImageName:     container.Config.Image,
			Tag:           tag,
			Host:          os.Getenv("TUTUM_NODE_HOSTNAME"),
		}

		pump := dumper.NewPump(&containerInfo)
		dumper.containers[container.ID] = true

		go func() {
			if err := pump.Run(); err != nil {
				logp.Err("Pump error for container %s: %s", container.ID, err)
				panic(err)
			}

			dumper.mutex.Lock()
			defer dumper.mutex.Unlock()
			delete(dumper.containers, container.ID)
		}()
	}
	return nil
}
