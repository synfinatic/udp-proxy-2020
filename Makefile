EXTENSION ?=
DIST_DIR ?= dist/
GOOS ?= $(shell uname -s | tr "[:upper:]" "[:lower:]")
ARCH ?= $(shell uname -m)
BUILDINFOSDET ?=

DOCKER_REPO        := synfinatic
PROJECT_NAME       := udp-proxy-2020
PROJECT_VERSION    := $(shell git describe --tags 2>/dev/null $(git rev-list --tags --max-count=1))
PROJECT_VERSION    := 0.0.3
VERSION_PKG        := $(shell echo $(PROJECT_VERSION) | sed 's/^v//g')
ARCH               := x86_64
LICENSE            := GPLv3
URL                := https://github.com/$(DOCKER_REPO)/$(PROJECT_NAME)
DESCRIPTION        := UDP Proxy 2020: A bad hack for a stupid problem
BUILDINFOS         := ($(shell date +%FT%T%z)$(BUILDINFOSDET))
LDFLAGS            := '-X main.version=$(PROJECT_VERSION) -X main.buildinfos=$(BUILDINFOS)'
OUTPUT_NAME        := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-$(GOOS)-$(ARCH)$(EXTENSION)
FREEBSD_NAME       := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-$(ARCH)$(EXTENSION)
PFSENSE_NAME       := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-pfsense-$(ARCH)$(EXTENSION)

ALL: udp-proxy-2020

test: test-race vet unittest

cmd/version.go:
	@echo "package main" >cmd/version.go
	@echo "" >>cmd/version.go
	@echo "const Version = \"$(PROJECT_VERSION)\"" >>cmd/version.go
	@echo "const Description = \"$(DESCRIPTION)\"" >>cmd/version.go
	@echo "const URL = \"$(URL)\"" >>cmd/version.go

PHONY: run
run: cmd/version.go
	go run cmd/*.go

clean-all: vagrant-destroy clean

clean:
	rm -f dist/*
	rm cmd/version.go

clean-go:
	go clean -i -r -cache -modcache

udp-proxy-2020: $(OUTPUT_NAME)

$(OUTPUT_NAME): prepare
	go build -ldflags $(LDFLAGS) -o $(OUTPUT_NAME) cmd/*.go

.PHONY: build-race
build-race: prepare
	go build -race -ldflags $(LDFLAGS) -o $(OUTPUT_NAME) cmd/*.go

debug: prepare
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
prepare: cmd/version.go
	mkdir -p $(DIST_DIR)

.PHONY: fmt
fmt:
	cd cmd && go fmt *.go

## targets to build pfSense binary
.PHONY: vagrant-check
vagrant-check:
	@which vagrant >/dev/null || "Please install Vagrant: https://www.vagrantup.com"
	@which VBoxManage >/dev/null || "Please install VirtualBox: https://www.virtualbox.org"

pfsense: $(PFSENSE_NAME)

.PHONY: vagrant-scp
vagrant-scp: vagrant-check ## Install the vagrant scp plugin
	@if test `vagrant plugin list | grep -c vagrant-scp` -eq 0 ; then \
	    vagrant plugin install vagrant-scp ; fi

$(PFSENSE_NAME): vagrant-scp
	vagrant up && vagrant scp :$(PROJECT_NAME)/$(FREEBSD_NAME) $(PFSENSE_NAME)

vagrant-destroy:
	vagrant destroy -f
