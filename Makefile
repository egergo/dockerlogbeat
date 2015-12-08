SOURCES = $(wildcard *.go beat/*.go)

all: image

build: build/dockerlogbeat

build/dockerlogbeat: $(SOURCES)
	docker run --rm -e GO15VENDOREXPERIMENT=1 -v $(CURDIR):/go/src/github.com/egergo/dockerlogbeat -v $(CURDIR)/build:/go/bin golang:alpine go install github.com/egergo/dockerlogbeat

test: tmp/coverage.out

tmp/coverage.out: $(SOURCES)
	docker run --rm -e GO15VENDOREXPERIMENT=1 -v $(CURDIR)/tmp:/tmp -v $(CURDIR):/go/src/github.com/egergo/dockerlogbeat -v $(CURDIR)/build:/go/bin golang:alpine go test -coverprofile=/tmp/coverage.out github.com/egergo/dockerlogbeat/beat

cover: tmp/coverage.out
	go tool cover -html=$(CURDIR)/tmp/coverage.out

image: build/dockerlogbeat build/Dockerfile
	docker build -t dockerlogbeat:build $(CURDIR)/build

run: image
	docker run --rm -v $(CURDIR)/tmp:/tmp -e DLB=ignore -v $(CURDIR)/dockerlogbeat.yml:/etc/dockerlogbeat/dockerlogbeat.yml -v /var/run/docker.sock:/var/run/docker.sock dockerlogbeat:build -e -cpuprofile /tmp/cpu -memprofile /tmp/mem -v

clean:
	rm -f $(CURDIR)/build/dockerlogbeat

.PHONY: test cover clean all build default image run

