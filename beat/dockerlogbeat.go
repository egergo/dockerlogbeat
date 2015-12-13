package dockerlogbeat

import (
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/cfgfile"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/publisher"
	"github.com/fsouza/go-dockerclient"
)

type DockerLogBeat struct {
	spooler  *Spooler
	registry *Registry

	Events chan []*DockerLogEvent

	PublisherAlive chan struct{}
	Stopped        chan struct{}

	RootConfig *RootConfig
}

func New() *DockerLogBeat {
	return &DockerLogBeat{
		Events: make(chan []*DockerLogEvent),

		PublisherAlive: make(chan struct{}),
		Stopped:        make(chan struct{}),

		RootConfig: NewDefaultConfig(),
	}
}

func (dlb *DockerLogBeat) Config(b *beat.Beat) error {
	var err error

	if err = cfgfile.Read(dlb.RootConfig, ""); err != nil {
		logp.Err("Error reading configuration file: %v", err)
		return err
	}

	dlb.spooler, err = NewSpooler(dlb.Events, dlb.RootConfig.Config)
	if err != nil {
		return err
	}

	dlb.registry, err = NewRegistry(dlb.RootConfig.Config.RegistryFile)
	if err != nil {
		return err
	}

	return nil
}

func (dlb *DockerLogBeat) Setup(b *beat.Beat) error {
	return nil
}

func (dlb *DockerLogBeat) Run(b *beat.Beat) error {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		logp.Err("Cannot create Docker client: %s", err)
		return err
	}

	dumper := NewDumper(client, dlb.registry, dlb.spooler.C, dlb.RootConfig.Config)
	if err := dumper.StartEventChannel(); err != nil {
		logp.Err("Cannot start Docker event channel: %s", err)
		return err
	}
	if err := dumper.ScanContainers(); err != nil {
		logp.Err("Cannot scan Docker containers: %s", err)
		return err
	}

	go dlb.registry.Run()
	go dlb.publish(b)
	go dlb.spooler.Run()

	<-dlb.Stopped
	return nil
}

func (dlb *DockerLogBeat) Cleanup(b *beat.Beat) error {
	return nil
}

func (dlb *DockerLogBeat) Stop() {
	logp.Info("Stopping")

	done := make(chan struct{})

	go func() {
		dlb.spooler.Close()
		dlb.registry.Close()

		close(dlb.Events)
		<-dlb.PublisherAlive

		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		logp.Err("Cannot stop cleanly, events lost")
	}

	close(dlb.Stopped)
}

func (dlb *DockerLogBeat) publish(beat *beat.Beat) {
	defer close(dlb.PublisherAlive)

	logp.Info("Start sending events to output")
	defer logp.Info("Stop sending events to output")

	for events := range dlb.Events {
		pubEvents := make([]common.MapStr, 0, len(events))
		for _, event := range events {
			pubEvents = append(pubEvents, event.ToMapStr())
		}

		logp.Info("beat: publishing %v events", len(pubEvents))
		beat.Events.PublishEvents(pubEvents, publisher.Sync)
		logp.Info("beat: published %v events", len(pubEvents))

		logp.Info("beat: registering %v events", len(pubEvents))
		dlb.registry.C <- pubEvents
		logp.Info("beat: registered %v events", len(pubEvents))
	}
}
