package dockerlogbeat

import (
	"time"

	"github.com/elastic/beats/libbeat/logp"
)

type Spooler struct {
	Target chan []*DockerLogEvent

	Spool []*DockerLogEvent
	C     chan *DockerLogEvent

	Alive   chan struct{}
	Stopped chan struct{}

	NextFlushTime time.Time
	IdleTimeout   time.Duration
}

func NewSpooler(target chan []*DockerLogEvent, config *Config) (*Spooler, error) {
	spoolSize := config.SpoolSize
	if spoolSize == 0 {
		spoolSize = 1024
	}

	s := &Spooler{
		Alive:   make(chan struct{}),
		Stopped: make(chan struct{}),

		Spool: make([]*DockerLogEvent, 0, spoolSize),
		C:     make(chan *DockerLogEvent, 16),

		Target: target,
	}

	if config.IdleTimeout == "" {
		s.IdleTimeout = 5 * time.Second
	} else {
		var err error
		s.IdleTimeout, err = time.ParseDuration(config.IdleTimeout)
		if err != nil {
			logp.Warn("Failed to parse idle timeout duration '%s'. Error was: %v", config.IdleTimeout, err)
			return nil, err
		}
	}
	s.NextFlushTime = time.Now().Add(s.IdleTimeout)

	return s, nil
}

func (s *Spooler) Run() {
	defer close(s.Alive)

	ticker := time.NewTicker(s.IdleTimeout / 2)
	defer ticker.Stop()

	logp.Info("Starting spooler: spool_size: %v; idle_timeout: %s", cap(s.Spool), s.IdleTimeout)

	stopped := false
	for !stopped {
		select {
		case event := <-s.C:
			s.Spool = append(s.Spool, event)
			if len(s.Spool) == cap(s.Spool) {
				s.flush()
			}

		case <-ticker.C:
			if time.Now().After(s.NextFlushTime) {
				s.flush()
			}

		case <-s.Stopped:
			stopped = true
		}
	}

	s.flush()
	close(s.C)
}

func (s *Spooler) Close() {
	logp.Info("Stopping spooler")
	close(s.Stopped)
	<-s.Alive
}

func (s *Spooler) flush() {
	if len(s.Spool) > 0 {
		logp.Info("spooler: publishing %v events", len(s.Spool))
		s.Target <- CopyDleSlice(s.Spool)
		logp.Info("spooler: published %v events", len(s.Spool))
		s.Spool = s.Spool[:0]
	}
	s.NextFlushTime = time.Now().Add(s.IdleTimeout)
}
