EXTENSION ?=
DIST_DIR ?= dist/
GOOS ?= $(shell uname -s | tr "[:upper:]" "[:lower:]")
ARCH ?= $(shell uname -m)
BUILDINFOSDET ?=

DOCKER_REPO        := synfinatic
PROJECT_NAME       := udp-proxy-2020
PROJECT_VERSION    := $(shell git describe --tags 2>/dev/null $(git rev-list --tags --max-count=1))
PROJECT_VERSION    := 0.0.1
VERSION_PKG        := $(shell echo $(PROJECT_VERSION) | sed 's/^v//g')
ARCH               := x86_64
LICENSE            := GPLv3
URL                := https://github.com/$(DOCKER_REPO)/$(PROJECT_NAME)
DESCRIPTION        := UDP Proxy 2020: A bad hack for a stupid problem
BUILDINFOS         := ($(shell date +%FT%T%z)$(BUILDINFOSDET))
LDFLAGS            := '-X main.version=$(PROJECT_VERSION) -X main.buildinfos=$(BUILDINFOS)'
OUTPUT_NAME        := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-$(GOOS)-$(ARCH)$(EXTENSION)

ALL: udp-proxy-2020

test: test-race vet unittest

PHONY: run
run:
	go run cmd/*.go

clean:
	rm -rf dist

clean-go:
	go clean -i -r -cache -modcache

udp-proxy-2020: $(OUTPUT_NAME)

$(OUTPUT_NAME): prepare
	go build -ldflags $(LDFLAGS) -o $(OUTPUT_NAME) cmd/*.go

.PHONY: build-race
build-race: prepare
	go build -race -ldflags $(LDFLAGS) -o $(OUTPUT_NAME) cmd/*.go

debug:
	dlv debug cmd/*.go

PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_REPO)/$(PROJECT_NAME):latest .
	docker run --rm \
	    --volume $(shell pwd)/dist:/go/dist \
	    $(DOCKER_REPO)/$(PROJECT_NAME):latest

PHONY: docker-clean
docker-clean:
	docker image rm $(DOCKER_REPO)/$(PROJECT_NAME):latest

PHONY: docker-shell
docker-shell:
	docker run -it --rm  \
	    --volume $(shell pwd)/dist:/go/dist \
	    $(DOCKER_REPO)/$(PROJECT_NAME):latest /bin/ash

.PHONY: unittest
unittest:
	go test ./...

.PHONY: test-race
test-race:
	@echo checking code for races...
	go test -race ./...

.PHONY: vet
vet:
	@echo checking code is vetted...
	go vet $(shell go list ./...)

.PHONY: prepare
prepare:
	mkdir -p $(DIST_DIR)

.PHONY: fmt
fmt:
	cd cmd && go fmt *.go
