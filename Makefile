DIST_DIR ?= dist/
GOOS ?= $(shell uname -s | tr "[:upper:]" "[:lower:]")
GOARCH ?= $(shell uname -m | sed -E 's/x86_64/amd64/')
BUILDINFOSDET ?=
UDP_PROXY_2020_ARGS ?=

PROJECT_VERSION    := 0.0.11
DOCKER_REPO        := synfinatic
PROJECT_NAME       := udp-proxy-2020
PROJECT_TAG        := $(shell git describe --tags 2>/dev/null $(git rev-list --tags --max-count=1))
ifeq ($(PROJECT_TAG),)
PROJECT_TAG        := NO-TAG
endif
PROJECT_COMMIT     := $(shell git rev-parse HEAD)
ifeq ($(PROJECT_COMMIT),)
PROJECT_COMMIT     := NO-CommitID
endif
PROJECT_DELTA      := $(shell DELTA_LINES=$$(git diff | wc -l); if [ $${DELTA_LINES} -ne 0 ]; then echo $${DELTA_LINES} ; else echo "''" ; fi)
VERSION_PKG        := $(shell echo $(PROJECT_VERSION) | sed 's/^v//g')
LICENSE            := GPLv3
URL                := https://github.com/$(DOCKER_REPO)/$(PROJECT_NAME)
DESCRIPTION        := UDP Proxy 2020: A bad hack for a stupid problem
BUILDINFOS         := $(shell date +%FT%T%z)$(BUILDINFOSDET)
HOSTNAME           := $(shell hostname)
LDFLAGS            := -X "main.Version=$(PROJECT_VERSION)" -X "main.Delta=$(PROJECT_DELTA)"
LDFLAGS            += -X "main.Buildinfos=$(BUILDINFOS)" -X "main.Tag=$(PROJECT_TAG)"
LDFLAGS            += -X "main.CommitID=$(PROJECT_COMMIT)" -s -w
OUTPUT_NAME        := $(DIST_DIR)$(PROJECT_NAME)-$(GOOS)-$(GOARCH)
DOCKER_VERSION     ?= v$(PROJECT_VERSION)

ALL: $(OUTPUT_NAME)

include help.mk  # place after ALL target and before all other targets

release: build-release ## Build and sign official release
	cd dist && shasum -a 256 udp-proxy-2020* | gpg --clear-sign >release.sig
	@echo "Now run `make docker-release`?"

build-release: clean linux-amd64 linux-mips64 linux-arm darwin-amd64 freebsd docker package ## Build our release binaries

tags: cmd/*.go  ## Create tags file for vim, etc
	@echo Make sure you have Universal Ctags installed: https://github.com/universal-ctags/ctags
	ctags -R

.PHONY: run
run: cmd/*.go ## build and run udp-proxy-2020 using $UDP_PROXY_2020_ARGS
	sudo go run cmd/*.go $(UDP_PROXY_2020_ARGS)

clean-all: freebsd-clean docker-clean clean ## Clean _everything_

clean: ## Remove all binaries in dist
	rm -f dist/*

clean-go: ## Clean Go cache
	go clean -i -r -cache -modcache

$(OUTPUT_NAME): cmd/*.go .prepare
	go build -ldflags='$(LDFLAGS)' -o $(OUTPUT_NAME) ./cmd
	@echo "Created: $(OUTPUT_NAME)"

.PHONY: build-race
build-race: .prepare ## Build race detection binary
	go build -race -ldflags='$(LDFLAGS)' -o $(OUTPUT_NAME) ./cmd

debug: .prepare ## Run debug in dlv
	dlv debug ./cmd

.PHONY: unittest
unittest: ## Run go unit tests
	go test ./...

.PHONY: test-race
test-race: ## Run `go test -race` on the code
	@echo checking code for races...
	go test -race ./...

.PHONY: vet
vet: ## Run `go vet` on the code
	@echo checking code is vetted...
	go vet $(shell go list ./...)

test: vet unittest ## Run all tests

.prepare: $(DIST_DIR)

$(DIST_DIR):
	@mkdir -p $(DIST_DIR)

.PHONY: fmt
fmt: ## Format Go code
	@go fmt cmd

.PHONY: test-fmt
test-fmt: fmt ## Test to make sure code if formatted correctly
	@if test `git diff cmd | wc -l` -gt 0; then \
	    echo "Code changes detected when running 'go fmt':" ; \
	    git diff -Xfiles ; \
	    exit -1 ; \
	fi

.PHONY: test-tidy
test-tidy: ## Test to make sure go.mod is tidy
	@go mod tidy
	@if test `git diff go.mod | wc -l` -gt 0; then \
	    echo "Need to run 'go mod tidy' to clean up go.mod" ; \
	    exit -1 ; \
	fi

precheck: test test-fmt test-tidy lint ## Run all tests that happen in a PR

lint:  ## Run golangci-lint
	golangci-lint run

######################################################################
# Linux targets for building Linux in Docker
######################################################################
LINUX_AMD64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-linux-amd64
AMD64_IMAGE 	   := $(DOCKER_REPO)/$(PROJECT_NAME)-builder-amd64:$(DOCKER_VERSION)

.PHONY: linux-amd64
linux-amd64: ## Build static Linux/x86_64 binary using Docker
	docker build -t $(AMD64_IMAGE) -f Dockerfile.amd64 .
	docker run --rm \
	    --volume $(shell pwd)/dist:/build/$(PROJECT_NAME)/dist \
	    $(AMD64_IMAGE)

.PHONY: linux-amd64-shell
linux-amd64-shell: ## Get a shell in Linux/x86_64 Docker container
	docker run -it --rm  \
	    --volume $(shell pwd)/dist:/build/$(PROJECT_NAME)/dist \
	    $(AMD64_IMAGE) /bin/bash

.linux-amd64: $(LINUX_AMD64_S_NAME)
$(LINUX_AMD64_S_NAME): .prepare
	LDFLAGS='-l/usr/lib/libpcap.a' CGO_ENABLED=1 \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' -o $(LINUX_AMD64_S_NAME) ./cmd
	@echo "Created: $(LINUX_AMD64_S_NAME)"

######################################################################
# Vagrant targets for building for FreeBSD/pfSense
######################################################################
.PHONY: .vagrant-check
.vagrant-check:
	@which vagrant >/dev/null || "Please install Vagrant: https://www.vagrantup.com"
	@which VBoxManage >/dev/null || "Please install VirtualBox: https://www.virtualbox.org"

freebsd: .vagrant-check ## Build all FreeBSD/pfSense binaries using Vagrant VM
	vagrant provision && vagrant up && vagrant ssh-config >.vagrant-ssh && \
		scp -F .vagrant-ssh default:$(PROJECT_NAME)/dist/*freebsd* dist/

freebsd-shell: ## Get a shell in FreeBSD Vagrant VM
	vagrant ssh

freebsd-clean: ## Destroy FreeBSD Vagrant VM
	vagrant destroy -f || true
	rm -f .vagrant-ssh

ifeq ($(GOOS),freebsd)
# FreeBSD aarch64, armv6 and armv7 targets only work inside of FreeBSD Vagrant VM
FREEBSD_AMD64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-amd64
FREEBSD_ARM64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-arm64
FREEBSD_ARMV6_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-armv6
FREEBSD_ARMV7_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-armv7

freebsd-binaries: freebsd-amd64 freebsd-arm64 freebsd-armv6 freebsd-armv7 ## no-help
freebsd-amd64: $(FREEBSD_AMD64_S_NAME) ## no-help
freebsd-arm64: $(FREEBSD_ARM64_S_NAME) ## no-help
freebsd-armv6: $(FREEBSD_ARMV6_S_NAME) ## no-help
freebsd-armv7: $(FREEBSD_ARMV7_S_NAME) ## no-help

# Seems to be a bug with CGO & Clang where it always wants to use the host arch
# linker and it doesn't seem to honor the LD ENV var :(
.PHONY: .freebsd-arm-cross .freebsd-amd64-cross .freebsd-aarch64-cross
.freebsd-aarch64-cross:
	@cd /usr/local/bin && \
		if test ! -f x86_64-unknown-freebsd12.2-ld.bfd.bak ; then \
			mv x86_64-unknown-freebsd12.2-ld.bfd x86_64-unknown-freebsd12.2-ld.bfd.bak ; \
			ln -s aarch64-unknown-freebsd12.2-ld.bfd x86_64-unknown-freebsd12.2-ld.bfd ; \
		fi

.freebsd-arm-cross:
	@cd /usr/local/bin && \
		if test ! -f x86_64-unknown-freebsd12.2-ld.bfd.bak ; then \
			mv x86_64-unknown-freebsd12.2-ld.bfd x86_64-unknown-freebsd12.2-ld.bfd.bak ; \
			ln -s arm-gnueabi-freebsd12.2-ld.bfd x86_64-unknown-freebsd12.2-ld.bfd ; \
		fi

.freebsd-amd64-cross:
	@cd /usr/local/bin && \
		if test -f x86_64-unknown-freebsd12.2-ld.bfd.bak ; then \
			rm x86_64-unknown-freebsd12.2-ld.bfd ; \
			mv x86_64-unknown-freebsd12.2-ld.bfd.bak x86_64-unknown-freebsd12.2-ld.bfd ;\
		fi

$(FREEBSD_AMD64_S_NAME): .freebsd-amd64-cross
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=1 \
	CGO_LDFLAGS='-libverbs' \
	go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
	-o $(FREEBSD_AMD64_S_NAME) cmd/*.go
	@echo "Created: $(FREEBSD_AMD64_S_NAME)"

$(FREEBSD_ARM64_S_NAME): .freebsd-aarch64-cross
	GOOS=freebsd GOARCH=arm64 CGO_ENABLED=1 \
	CGO_LDFLAGS='--sysroot=/usr/local/freebsd-sysroot/aarch64 -libverbs' \
	CGO_CFLAGS='-I/usr/local/freebsd-sysroot/aarch64/usr/include' \
	CC=/usr/local/freebsd-sysroot/aarch64/bin/cc \
	PKG_CONFIG_PATH=/usr/local/freebsd-sysroot/aarch64/usr/libdata/pkgconfig \
	go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
	-o $(FREEBSD_ARM64_S_NAME) cmd/*.go
	@echo "Created: $(FREEBSD_ARM64_S_NAME)"

$(FREEBSD_ARMV6_S_NAME): .freebsd-arm-cross
	GOOS=freebsd GOARCH=arm GOARM=6 CGO_ENABLED=1 \
	CGO_LDFLAGS='--sysroot=/usr/local/freebsd-sysroot/armv6 -libverbs' \
	CGO_CFLAGS='-I/usr/local/freebsd-sysroot/armv6/usr/include' \
	CC=/usr/local/freebsd-sysroot/armv6/bin/cc \
	PKG_CONFIG_PATH=/usr/local/freebsd-sysroot/armv6/usr/libdata/pkgconfig \
	go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
	-o $(FREEBSD_ARMV6_S_NAME) cmd/*.go
	@echo "Created: $(FREEBSD_ARMV6_S_NAME)"

$(FREEBSD_ARMV7_S_NAME): .freebsd-arm-cross
	GOOS=freebsd GOARCH=arm GOARM=7 CGO_ENABLED=1 \
	CGO_LDFLAGS='--sysroot=/usr/local/freebsd-sysroot/armv7 -libverbs' \
	CGO_CFLAGS='-I/usr/local/freebsd-sysroot/armv7/usr/include' \
	CC=/usr/local/freebsd-sysroot/armv7/bin/cc \
	PKG_CONFIG_PATH=/usr/local/freebsd-sysroot/armv7/usr/libdata/pkgconfig \
	go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
	-o $(FREEBSD_ARMV7_S_NAME) cmd/*.go
	@echo "Created: $(FREEBSD_ARMV7_S_NAME)"
endif

######################################################################
# MIPS64 targets for building for Ubiquiti USG/Edgerouter
######################################################################
LINUX_MIPS64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-linux-mips64
MIPS64_IMAGE 	    := $(DOCKER_REPO)/$(PROJECT_NAME)-builder-mips64:$(DOCKER_VERSION)

.PHONY: linux-mips64
linux-mips64: .prepare ## Build Linux/MIPS64 static binary in Docker container
	docker build -t $(MIPS64_IMAGE) -f Dockerfile.mips64 .
	docker run --rm \
	    --volume $(shell pwd):/build/udp-proxy-2020 \
	    $(MIPS64_IMAGE)

.PHONY: linux-mips64-shell
linux-mips64-shell: .prepare ## Get a shell in Linux/MIPS64 build Docker container
	docker run -it --rm \
	    --volume $(shell pwd):/build/udp-proxy-2020 \
	    --entrypoint /bin/bash $(MIPS64_IMAGE)

.linux-mips64: $(LINUX_MIPS64_S_NAME)
$(LINUX_MIPS64_S_NAME): .prepare
	LDFLAGS='-l/usr/mips64-linux-gnuabi64/lib/libpcap.a' \
	    GOOS=linux GOARCH=mips64 CGO_ENABLED=1 CC=mips64-linux-gnuabi64-gcc \
	    PKG_CONFIG_PATH=/usr/mips64-linux-gnuabi64/lib/pkgconfig \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' -o $(LINUX_MIPS64_S_NAME) cmd/*.go
	@echo "Created: $(LINUX_MIPS64_S_NAME)"

######################################################################
# Targets for building for Linux/ARM32 no hardware floating point
######################################################################
LINUX_ARMV5_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-linux-armv5
LINUX_ARMV6_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-linux-armv6
LINUX_ARMV7_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-linux-armv7
LINUX_ARM64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-linux-arm64
ARM_IMAGE 	   := $(DOCKER_REPO)/$(PROJECT_NAME)-builder-arm:$(DOCKER_VERSION)

.PHONY: linux-arm
linux-arm: .prepare ## Build Linux/arm static binaries in Docker container
	docker build -t $(ARM_IMAGE) -f Dockerfile.arm .
	docker run --rm \
	    --volume $(shell pwd):/build/udp-proxy-2020 \
	    $(ARM_IMAGE)

.PHONY: linux-arm-shell
linux-arm-shell: .prepare ## Get a shell in Linux/arm build Docker container
	docker run -it --rm \
	    --volume $(shell pwd):/build/udp-proxy-2020 \
	    --entrypoint /bin/bash $(ARM_IMAGE)

.linux-arm: $(LINUX_ARMV5_S_NAME) $(LINUX_ARMV6_S_NAME) $(LINUX_ARMV7_S_NAME) $(LINUX_ARM64_S_NAME)
$(LINUX_ARMV5_S_NAME): .prepare
	LDFLAGS='-l/usr/arm-linux-gnueabi/lib/libpcap.a' \
	    GOOS=linux GOARCH=arm GOARM=5 CGO_ENABLED=1 CC=arm-linux-gnueabi-gcc-10 \
	    PKG_CONFIG_PATH=/usr/arm-linux-gnueabi/lib/pkgconfig \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' -o $(LINUX_ARMV5_S_NAME) cmd/*.go
	@echo "Created: $(LINUX_ARMV5_S_NAME)"

$(LINUX_ARMV6_S_NAME): .prepare
	LDFLAGS='-l/usr/arm-linux-gnueabi/lib/libpcap.a' \
	    GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=1 CC=arm-linux-gnueabihf-gcc-10 \
	    PKG_CONFIG_PATH=/usr/arm-linux-gnueabihf/lib/pkgconfig \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' -o $(LINUX_ARMV6_S_NAME) cmd/*.go
	@echo "Created: $(LINUX_ARMV6_S_NAME)"

$(LINUX_ARMV7_S_NAME): .prepare
	LDFLAGS='-l/usr/arm-linux-gnueabi/lib/libpcap.a' \
	    GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 CC=arm-linux-gnueabihf-gcc-10 \
	    PKG_CONFIG_PATH=/usr/arm-linux-gnueabihf/lib/pkgconfig \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' -o $(LINUX_ARMV7_S_NAME) cmd/*.go
	@echo "Created: $(LINUX_ARMV7_S_NAME)"

$(LINUX_ARM64_S_NAME): .prepare
	LDFLAGS='-l/usr/aarch64-linux-gnu/lib/libpcap.a' \
	    GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc-10 \
	    PKG_CONFIG_PATH=/usr/aarch64-linux-gnu/lib/pkgconfig \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' -o $(LINUX_ARM64_S_NAME) cmd/*.go
	@echo "Created: $(LINUX_ARM64_S_NAME)"

######################################################################
# Targets for building macOS/Darwin (only valid on macOS)
######################################################################
ifeq ($(GOOS),darwin)
DARWIN_AMD64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-darwin-amd64
darwin-amd64: $(DARWIN_AMD64_S_NAME) ## Build macOS/amd64 binary

$(DARWIN_AMD64_S_NAME): cmd/*.go .prepare
	GOOS=darwin GOARCH=amd64 go build -ldflags='$(LDFLAGS)' \
	     -o $(DARWIN_AMD64_S_NAME) cmd/*.go
	@echo "Created: $(DARWIN_AMD64_S_NAME)"
endif

######################################################################
# Docker image for running in docker container for UDM Pro/etc
######################################################################
.PHONY: docker docker-clean .docker
docker: ## Build docker image for Linux/amd64
	docker build \
	    -t $(DOCKER_REPO)/$(PROJECT_NAME):$(DOCKER_VERSION) \
	    --build-arg VERSION=$(DOCKER_VERSION) \
	    -f Dockerfile .

.docker:
	CGO_ENABLED=1 \
	go build -ldflags '$(LDFLAGS)' -o dist/udp-proxy-2020 cmd/*.go

docker-shell: ## Get a shell in the docker image
	docker run --rm -it --network=host \
	    $(DOCKER_REPO)/$(PROJECT_NAME):$(DOCKER_VERSION) \
	    /bin/sh

docker-release: ## Tag and push docker images Linux AMD64/ARM64
	docker buildx build \
	    -t $(DOCKER_REPO)/$(PROJECT_NAME):$(DOCKER_VERSION) \
	    -t $(DOCKER_REPO)/$(PROJECT_NAME):latest \
	    --build-arg VERSION=$(DOCKER_VERSION) \
	    --platform linux/arm64,linux/amd64 \
	    --push -f Dockerfile .

docker-clean: ## Remove all docker build images
	docker image rm $(ARM_IMAGE) $(AMD64_IMAGE) $(MIPS64_IMAGE) || true

package: linux-amd64 linux-arm  ## Build deb/rpm packages
	docker build -t udp-proxy-2020-builder:latest -f Dockerfile.package .
	docker run --rm \
		-v $$(pwd)/dist:/root/dist \
		-e VERSION=$(PROJECT_VERSION) udp-proxy-2020-builder:latest
