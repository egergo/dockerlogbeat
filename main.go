package main

import (
	"github.com/egergo/dockerlogbeat/beat"
	"github.com/elastic/beats/libbeat/beat"
)

var Version = "0.1.0"
var Name = "dockerlogbeat"

func main() {
	beat.Run(Name, Version, dockerlogbeat.New())
}
