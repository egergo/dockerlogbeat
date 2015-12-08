package dockerlogbeat

import (
	"github.com/elastic/beats/libbeat/logp"
	"github.com/fsouza/go-dockerclient"
)

type Pump struct {
	Dumper    *Dumper
	Container *ContainerInfo
}

type PumpWriter struct {
	Pump   *Pump
	StdErr bool
}

func (dumper *Dumper) NewPump(container *ContainerInfo) *Pump {
	return &Pump{
		Dumper:    dumper,
		Container: container,
	}
}

func (pump *Pump) Run() error {
	logp.Info("Pump started for %s", pump.Container.ContainerName)
	defer logp.Info("Pump stopped for %s", pump.Container.ContainerName)

	stdoutWriter := PumpWriter{
		Pump: pump,
	}

	stderrWriter := PumpWriter{
		Pump:   pump,
		StdErr: true,
	}

	return pump.Dumper.DockerClient.Logs(docker.LogsOptions{
		Container:    pump.Container.ContainerID,
		Stdout:       true,
		Stderr:       true,
		Follow:       true,
		Timestamps:   true,
		OutputStream: &stdoutWriter,
		ErrorStream:  &stderrWriter,
		Since:        pump.Dumper.Registry.GetLast(pump.Container.ContainerID),
	})
}

func (pw *PumpWriter) Write(p []byte) (n int, err error) {
	line := CopySlice(p)
	pw.Pump.Dumper.Target <- &DockerLogEvent{
		Line:      line,
		StdErr:    pw.StdErr,
		Container: pw.Pump.Container,
	}
	return len(p), nil
}
