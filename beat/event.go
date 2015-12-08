package dockerlogbeat

import (
	"time"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

type ContainerInfo struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	ImageID       string `json:"image_id"`
	ImageName     string `json:"image_name"`
	Tag           string `json:"tag,omitempty"`
	Host          string `json:"host,omitempty"`
}

type DockerLogEvent struct {
	Line      []byte
	StdErr    bool
	Container *ContainerInfo
}

func (event *DockerLogEvent) ToMapStr() common.MapStr {

	ts := string(event.Line[:30])
	t, err := time.Parse("2006-01-02T15:04:05Z", ts)
	if err != nil {
		logp.Err("Cannot parse timestamp: %v %v", ts, err)
	}

	id := event.Container.Host + "/" + event.Container.ContainerID + "/" + ts

	return common.MapStr{
		"@timestamp": common.Time(t),
		"type":       "dockerlogbeat",
		"id":         id,
		"message":    string(event.Line[31 : len(event.Line)-1]),
		"docker":     event.Container,
		"stderr":     event.StdErr,
	}
}
