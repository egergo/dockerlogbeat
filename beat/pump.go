package dockerlogbeat

import (
	"io"
	"regexp"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/fsouza/go-dockerclient"
)

type Pump struct {
	Dumper          *Dumper
	Container       *ContainerInfo
	MultilineRegexp *regexp.Regexp
}

type PumpWriter struct {
	Pump   *Pump
	StdErr bool
}

type MultilinePumpWriter struct {
	PumpWriter
	currentLine []byte
}

func (dumper *Dumper) NewPump(container *ContainerInfo, multilineRegexp *regexp.Regexp) *Pump {
	return &Pump{
		Dumper:          dumper,
		Container:       container,
		MultilineRegexp: multilineRegexp,
	}
}

func (pump *Pump) Run() error {
	logp.Info("Pump started for %s", pump.Container.ContainerName)
	defer logp.Info("Pump stopped for %s", pump.Container.ContainerName)

	var stdoutWriter, stderrWriter io.WriteCloser

	if pump.MultilineRegexp != nil {
		stdoutWriter = &MultilinePumpWriter{
			PumpWriter: PumpWriter{
				Pump: pump,
			},
		}
		stderrWriter = &MultilinePumpWriter{
			PumpWriter: PumpWriter{
				Pump:   pump,
				StdErr: true,
			},
		}
	} else {
		stdoutWriter = &PumpWriter{
			Pump: pump,
		}
		stderrWriter = &PumpWriter{
			Pump:   pump,
			StdErr: true,
		}
	}

	err := pump.Dumper.DockerClient.Logs(docker.LogsOptions{
		Container:    pump.Container.ContainerID,
		Stdout:       true,
		Stderr:       true,
		Follow:       true,
		Timestamps:   true,
		OutputStream: stdoutWriter,
		ErrorStream:  stderrWriter,
		Since:        pump.Dumper.Registry.GetLast(pump.Container.ContainerID),
	})

	stdoutWriter.Close()
	stderrWriter.Close()

	return err
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

func (pw *PumpWriter) Close() error {
	return nil
}

func (pw *MultilinePumpWriter) Write(p []byte) (n int, err error) {
	newLine := !pw.Pump.MultilineRegexp.Match(p[31:])
	if newLine {
		pw.pumpCurrent()
		pw.currentLine = CopySlice(p)
	} else {
		pw.currentLine = append(pw.currentLine, p[31:]...)
	}
	return len(p), nil
}

func (pw *MultilinePumpWriter) Close() error {
	pw.pumpCurrent()
	return nil
}

func (pw *MultilinePumpWriter) pumpCurrent() {
	if pw.currentLine == nil {
		return
	}
	pw.Pump.Dumper.Target <- &DockerLogEvent{
		Line:      pw.currentLine,
		StdErr:    pw.StdErr,
		Container: pw.Pump.Container,
	}
}
