package dockerlogbeat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

type State struct {
	Containers map[string]ContainerState `json:"containers"`
}

type ContainerState struct {
	Last int64 `json:"last"`
}

type Registry struct {
	file  string
	state State
	mutex sync.RWMutex

	C chan []common.MapStr

	Stopped chan struct{}
	Alive   chan struct{}
}

func NewRegistry(path string) (*Registry, error) {
	if path == "" {
		path = ".dockerlogbeat"
	}

	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to get the absolute path of %s: %v", path, err)
	}

	dir := filepath.Dir(path)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("Failed to created registry file dir %s: %v", path, err)
	}

	logp.Info("Registry file: %s", path)

	registry := Registry{
		file: path,

		C: make(chan []common.MapStr),

		Stopped: make(chan struct{}),
		Alive:   make(chan struct{}),
	}

	if existing, e := os.Open(path); e == nil {
		defer existing.Close()
		logp.Info("Loading registrar data from %s", path)
		decoder := json.NewDecoder(existing)
		decoder.Decode(&registry.state)
	}

	if registry.state.Containers == nil {
		registry.state.Containers = make(map[string]ContainerState)
	}

	return &registry, nil
}

func (registry *Registry) Run() {
	defer close(registry.Alive)

	stopped := false
	for !stopped {
		select {
		case <-registry.Stopped:
			stopped = true

		case events := <-registry.C:
			if err := registry.processEvents(events); err != nil {
				panic(err)
			}
		}
	}
}

func (registry *Registry) GetLast(container string) int64 {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()

	return registry.state.Containers[container].Last
}

func (registry *Registry) processEvents(events []common.MapStr) error {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	for _, event := range events {
		time := time.Time(event["@timestamp"].(common.Time))
		container := event["docker"].(*ContainerInfo).ContainerID

		unix := time.Unix()
		if registry.state.Containers[container].Last < unix {
			temp := registry.state.Containers[container]
			temp.Last = unix
			registry.state.Containers[container] = temp
		}
	}

	return registry.write()
}

func (registry *Registry) write() error {
	logp.Debug("registry", "Write registry file: %s", registry.file)

	tempfile := registry.file + ".new"
	file, err := os.Create(tempfile)
	if err != nil {
		logp.Err("Failed to create tempfile (%s) for writing: %s", tempfile, err)
		return err
	}

	encoder := json.NewEncoder(file)
	encoder.Encode(registry.state)
	file.Close()

	logp.Info("Registry file updated.")

	if err := os.Rename(tempfile, registry.file); err != nil {
		logp.Err("Rotate error: %s", err)
		return err
	}

	return nil
}

func (registry *Registry) Close() {
	close(registry.Stopped)
	<-registry.Alive
}
