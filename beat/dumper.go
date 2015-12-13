package dockerlogbeat

import (
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/fsouza/go-dockerclient"
)

var (
	EnvVarRegexp *regexp.Regexp
)

type Dumper struct {
	DockerClient *docker.Client
	Registry     *Registry
	Target       chan *DockerLogEvent
	Config       *Config

	containers map[string]bool
	mutex      sync.Mutex
	events     chan *docker.APIEvents
}

func init() {
	var err error
	EnvVarRegexp, err = regexp.Compile("^([^=]*)=(.*)")
	if err != nil {
		panic(err)
	}
}

func NewDumper(client *docker.Client, registry *Registry, target chan *DockerLogEvent, config *Config) *Dumper {
	dumper := Dumper{
		DockerClient: client,
		Registry:     registry,
		Target:       target,
		Config:       config,

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
		if dumper.Config.StopTutumLogrotate && strings.HasPrefix(c.Image, "tutum/logrotate") {
			go dumper.DockerClient.StopContainer(c.ID, 0)
			continue
		}

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

		envvars := make(map[string]string)
		for _, line := range container.Config.Env {
			res := EnvVarRegexp.FindStringSubmatch(line)
			if res == nil {
				logp.Warn("Cannot parse env var: %v", line)
				continue
			}
			envvars[res[1]] = res[2]
		}

		if envvars["LOGSPOUT"] == "ignore" || envvars["DLB"] == "ignore" {
			continue
		}

		tag := envvars["DLB_TAG"]

		pattern := envvars["DLB_MULTILINE_PATTERN"]
		var multilineRegexp *regexp.Regexp
		if pattern != "" {
			var err error
			multilineRegexp, err = regexp.Compile(pattern)
			if err != nil {
				logp.Warn("Cannot compile multiline regexp: %v %v", pattern, err)
				continue
			}
		}

		negate := envvars["DLB_MULTILINE_NEGATE"]
		multilineNegate := false
		if negate == "true" || negate == "1" {
			multilineNegate = true
		}

		containerInfo := ContainerInfo{
			ContainerID:   container.ID,
			ContainerName: container.Name[1:],
			ImageID:       container.Image,
			ImageName:     container.Config.Image,
			Tag:           tag,
			Host:          os.Getenv("TUTUM_NODE_HOSTNAME"),
		}

		pump := dumper.NewPump(&containerInfo, multilineRegexp, multilineNegate)
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
