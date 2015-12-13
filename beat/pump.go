package dockerlogbeat

import (
	"io"
	"regexp"
	"sync"
	"time"

	"github.com/elastic/beats/libbeat/logp"
	"github.com/fsouza/go-dockerclient"
)

type Pump struct {
	Dumper          *Dumper
	Container       *ContainerInfo
	MultilineRegexp *regexp.Regexp
	MultilineNegate bool
}

type PumpWriter struct {
	Pump   *Pump
	StdErr bool
}

type MultilinePumpWriter struct {
	PumpWriter

	currentLine   []byte
	ticker        *time.Ticker
	nextFlushTime time.Time
	mutex         sync.Mutex

	Stopped chan struct{}
}

func (dumper *Dumper) NewPump(container *ContainerInfo, multilineRegexp *regexp.Regexp, multilineNegate bool) *Pump {
	return &Pump{
		Dumper:          dumper,
		Container:       container,
		MultilineRegexp: multilineRegexp,
		MultilineNegate: multilineNegate,
	}
}

func (pump *Pump) Run() error {
	logp.Info("Pump started for %s", pump.Container.ContainerName)
	defer logp.Info("Pump stopped for %s", pump.Container.ContainerName)

	var stdoutWriter, stderrWriter io.WriteCloser

	if pump.MultilineRegexp != nil {
		stdoutWriter = NewMultilinePumpWriter(pump, false)
		stderrWriter = NewMultilinePumpWriter(pump, true)
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

func NewMultilinePumpWriter(pump *Pump, stdErr bool) *MultilinePumpWriter {
	pw := &MultilinePumpWriter{
		PumpWriter: PumpWriter{
			Pump:   pump,
			StdErr: stdErr,
		},
		ticker:  time.NewTicker(2 * time.Second),
		Stopped: make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-pw.Stopped:
				return
			case <-pw.ticker.C:
				if pw.currentLine != nil && time.Now().After(pw.nextFlushTime) {
					pw.update(true, nil)
				}
			}
		}
	}()

	return pw
}

func (pw *MultilinePumpWriter) Write(p []byte) (n int, err error) {
	flushCurrent := !pw.Pump.MultilineRegexp.Match(p[31:])
	if pw.Pump.MultilineNegate {
		flushCurrent = !flushCurrent
	}
	pw.update(flushCurrent, p)
	return len(p), nil
}

func (pw *MultilinePumpWriter) Close() error {
	pw.ticker.Stop()
	close(pw.Stopped)

	pw.update(true, nil)
	return nil
}

func (pw *MultilinePumpWriter) update(flushCurrent bool, incoming []byte) {
	pw.mutex.Lock()
	defer pw.mutex.Unlock()

	if flushCurrent {
		if pw.currentLine != nil {
			pw.Pump.Dumper.Target <- &DockerLogEvent{
				Line:      pw.currentLine,
				StdErr:    pw.StdErr,
				Container: pw.Pump.Container,
			}
		}
		pw.currentLine = CopySlice(incoming)
	} else {
		pw.currentLine = append(pw.currentLine, incoming[31:]...)
	}
	pw.nextFlushTime = time.Now().Add(3 * time.Second)
}
