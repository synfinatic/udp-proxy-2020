DIST_DIR ?= dist/
GOOS ?= $(shell uname -s | tr "[:upper:]" "[:lower:]")
GOARCH ?= $(shell uname -m | sed -E 's/x86_64/amd64/')
BUILDINFOSDET ?=
UDP_PROXY_2020_ARGS ?=

PROJECT_VERSION    := 0.2.0
DOCKER_REPO        := synfinatic
PROJECT_NAME       := udp-proxy-2020
PROJECT_TAG        := $(shell git describe --tags 2>/dev/null $(git rev-list --tags --max-count=1))
ifeq ($(PROJECT_TAG),)
PROJECT_TAG        := NO-TAG
endif
PROJECT_COMMIT     := $(shell git rev-parse HEAD 2>/dev/null)
ifeq ($(PROJECT_COMMIT),)
PROJECT_COMMIT     := NO-CommitID
endif
PROJECT_DELTA      := $(shell DELTA_LINES=$$(git diff 2>/dev/null | wc -l); if [ $${DELTA_LINES} -ne 0 ]; then echo $${DELTA_LINES} ; else echo "''" ; fi)
VERSION_PKG        := $(shell echo $(PROJECT_VERSION) | sed 's/^v//g')
LICENSE            := GPLv3
URL                := https://github.com/$(DOCKER_REPO)/$(PROJECT_NAME)
DESCRIPTION        := UDP Proxy 2020: A bad hack for a stupid problem
BUILDINFOS         := $(shell date +%FT%T%z)$(BUILDINFOSDET)
HOSTNAME           := $(shell hostname)
LDFLAGS            := $(LDFLAGS) -X "main.Version=$(PROJECT_VERSION)" -X "main.Delta=$(PROJECT_DELTA)"
LDFLAGS            += -X "main.Buildinfos=$(BUILDINFOS)" -X "main.Tag=$(PROJECT_TAG)"
LDFLAGS            += -X "main.CommitID=$(PROJECT_COMMIT)" -s -w
OUTPUT_NAME        := $(DIST_DIR)$(PROJECT_NAME)-$(GOOS)-$(GOARCH)
DOCKER_VERSION     ?= v$(PROJECT_VERSION)
FREEBSD_VERSION    := 14.2
FREEBSD_ARCHES     ?= amd64 arm64 # armv7 disabled
GOLANGCI_LINT_VERSION := 2.10.1

ALL: $(OUTPUT_NAME)

include help.mk  # place after ALL target and before all other targets

release: build-release ## Build and sign official release
	cd dist && shasum -a 256 udp-proxy-2020* | gpg --clear-sign >release.sig
	@echo "Now run `make docker-release`?"

build-release: clean linux-amd64 linux-mips64 linux-arm darwin-amd64 freebsd docker package ## Build our release binaries

tags: ./cmd/udp-proxy-2020/*.go  ## Create tags file for vim, etc
	@echo Make sure you have Universal Ctags installed: https://github.com/universal-ctags/ctags
	ctags -R

.PHONY: run
run: ./cmd/udp-proxy-2020/*.go ## build and run udp-proxy-2020 using $UDP_PROXY_2020_ARGS
	sudo go run ./cmd/udp-proxy-2020/... $(UDP_PROXY_2020_ARGS)

clean-all: freebsd-clean docker-clean clean ## Clean _everything_

clean: ## Remove all binaries in dist
	rm -f dist/*

clean-go: ## Clean Go cache
	go clean -i -r -cache -modcache

$(OUTPUT_NAME): ./cmd/udp-proxy-2020/*.go .prepare
	go build -ldflags='$(LDFLAGS)' -o $(OUTPUT_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(OUTPUT_NAME)"

.PHONY: build-race
build-race: .prepare ## Build race detection binary
	go build -race -ldflags='$(LDFLAGS)' -o $(OUTPUT_NAME) ./cmd/udp-proxy-2020/...

debug: .prepare ## Run debug in dlv
	dlv debug ./cmd

.PHONY: unittest
unittest: ## Run go unit tests
	@echo running unit tests...
	@go test ./...

.PHONY: test-race
test-race: ## Run `go test -race` on the code
	@echo checking code for races...
	@go test -race ./...

.PHONY: vet
vet: ## Run `go vet` on the code
	@echo checking code is vetted...
	@go vet $(shell go list ./...)

lint-install:  ## Install golangci-lint
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v$(GOLANGCI_LINT_VERSION)

.PHONY: .print-golangci-lint-version
.print-golangci-lint-version:  ## Print golangci-lint version
	@echo v$(GOLANGCI_LINT_VERSION)

.PHONY: .lint-check
.lint-check:
	@if test $$(golangci-lint --version 2>&1 | grep -c "version $(GOLANGCI_LINT_VERSION)") -eq 0 ; then \
		echo "Need to install golangci-lint $(GOLANGCI_LINT_VERSION)" ; \
		echo "Run: make lint-install" ; \
		exit -1 ; \
	fi

.PHONY: golangci-lint
golangci-lint: .lint-check ## Run golangci-lint on the code
	golangci-lint run

test: $(OUTPUT_NAME) vet unittest golangci-lint ## Run all tests

.PHONY: .build-test
.build-test: unittest vet test-fmt test-tidy

.prepare: $(DIST_DIR)

$(DIST_DIR):
	@mkdir -p $(DIST_DIR)

.PHONY: fmt
fmt: ## Format Go code
	@go fmt ./cmd/...

.PHONY: test-fmt
test-fmt: fmt ## Test to make sure code if formatted correctly
	@if test `git diff ./cmd | wc -l` -gt 0; then \
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
	docker run -it --rm  --entrypoint /bin/bash \
	    --volume $(shell pwd)/dist:/build/$(PROJECT_NAME)/dist \
	    $(AMD64_IMAGE)

#	CGO_LDFLAGS="$$(pkg-config --libs libpcap)" CGO_ENABLED=1 
.linux-amd64: $(LINUX_AMD64_S_NAME)
$(LINUX_AMD64_S_NAME): .prepare
	LDFLAGS="-l/usr/local/lib/libpcap.a" CGO_ENABLED=1 \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
	    	-o $(LINUX_AMD64_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(LINUX_AMD64_S_NAME)"

######################################################################
# Vagrant targets for building for FreeBSD
######################################################################
.vagrant-check:
	@which vagrant >/dev/null || { echo "Please install Vagrant: https://www.vagrantup.com"; exit 1; }
	@provider=$${VAGRANT_DEFAULT_PROVIDER:-virtualbox}; \
		if test "$$provider" = "virtualbox" ; then \
			which VBoxManage >/dev/null || { echo "Please install VirtualBox or set VAGRANT_DEFAULT_PROVIDER: https://www.virtualbox.org"; exit 1; }; \
		fi
	@touch .vagrant-check

freebsd: .vagrant-check ## Build all FreeBSD binaries using Vagrant
	@echo 'Run `vagrant provision` to reprovision the VM if you have made changes to the Vagrantfile or provisioning scripts.'
	@set -e; \
		mtime() { stat -f %m "$$1" 2>/dev/null || stat -c %Y "$$1"; }; \
		src_latest=$$( ( \
			find cmd internal -type f -name '*.go'; \
			printf '%s\n' go.mod go.sum Makefile Vagrantfile \
		) | while read -r f; do \
			test -f "$$f" && mtime "$$f"; \
		done | sort -nr | head -1); \
		test -n "$$src_latest" || src_latest=0; \
		arches=$$(echo "$(FREEBSD_ARCHES)" | sed -e 's/aarch64/arm64/g'); \
		needs_build=0; \
		for arch in $$arches; do \
			artifact="dist/$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-$$arch"; \
			if test ! -f "$$artifact"; then \
				echo "Missing artifact: $$artifact"; \
				needs_build=1; \
				break; \
			fi; \
			artifact_mtime=$$(mtime "$$artifact"); \
			if test "$$artifact_mtime" -lt "$$src_latest"; then \
				echo "Stale artifact: $$artifact"; \
				needs_build=1; \
				break; \
			fi; \
		done; \
		if test "$$needs_build" -eq 0; then \
			echo "FreeBSD artifacts are up-to-date for arches: $$arches"; \
			echo "Skipping Vagrant build"; \
		else \
			vm_state=$$(vagrant status --machine-readable | awk -F, '$$3=="state" {print $$4; exit}'); \
			if test "$$vm_state" = "not_created"; then \
				FREEBSD_ARCHES="$(FREEBSD_ARCHES)" FREEBSD_SKIP_TRIGGER_BUILD=1 vagrant up; \
			else \
				FREEBSD_ARCHES="$(FREEBSD_ARCHES)" FREEBSD_SKIP_TRIGGER_BUILD=1 vagrant up --no-provision; \
			fi; \
			vagrant rsync; \
			vagrant ssh -c 'sh -c "PATH=/usr/local/bin:$${PATH}; cd $(PROJECT_NAME); find cmd internal -type f -name '\''*.go'\'' -exec touch {} +; touch go.mod go.sum Makefile Vagrantfile 2>/dev/null || true; gmake --no-silent -B FREEBSD_ARCHES=\"$(FREEBSD_ARCHES)\" freebsd-binaries"'; \
			vagrant ssh-config >.vagrant-ssh; \
			scp -F .vagrant-ssh default:$(PROJECT_NAME)/dist/*freebsd* dist/; \
		fi

freebsd-shell: ## Get a shell in FreeBSD Vagrant VM
	vagrant ssh

freebsd-clean: ## Destroy FreeBSD Vagrant VM
	vagrant destroy -f || true
	rm -f .vagrant-ssh

ifeq ($(GOOS),freebsd)
# FreeBSD aarch64 and armv7 targets only work inside of FreeBSD Vagrant VM
FREEBSD_AMD64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-amd64
FREEBSD_ARM64_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-arm64
FREEBSD_ARMV7_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-armv7
FREEBSD_ARCHES_NORMALIZED := $(sort $(subst aarch64,arm64,$(FREEBSD_ARCHES)))
FREEBSD_ARCH_TARGETS := $(addprefix freebsd-,$(FREEBSD_ARCHES_NORMALIZED))

freebsd-binaries: $(FREEBSD_ARCH_TARGETS) ## no-help
	@for arch in $(FREEBSD_ARCHES_NORMALIZED); do \
		artifact="$(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-freebsd-$${arch}"; \
		test -f "$$artifact" || { echo "Missing expected FreeBSD artifact: $$artifact"; exit 1; }; \
	done
	@echo "Created FreeBSD artifacts for: $(FREEBSD_ARCHES_NORMALIZED)"

freebsd-amd64: $(FREEBSD_AMD64_S_NAME) ## no-help
freebsd-arm64: $(FREEBSD_ARM64_S_NAME) ## no-help
freebsd-aarch64: freebsd-arm64 ## no-help
freebsd-armv7: $(FREEBSD_ARMV7_S_NAME) ## no-help

# configure our build flags for cross or native compiling for FreeBSD based on our current GOARCH
ifeq ($(GOARCH),amd64)
AMD64_CGO_LDFLAGS := "$$(pkg-config --libs libpcap) -libverbs"
AMD64_CGO_CFLAGS := "$$(pkg-config --cflags libpcap)"
AMD64_CC := ""
ARM64_CGO_LDFLAGS := "$$(pkg-config --libs libpcap --define-variable=prefix=/usr/local/freebsd-sysroot/aarch64) -libverbs"
ARM64_CGO_CFLAGS := "$$(pkg-config --cflags libpcap --define-variable=prefix=/usr/local/freebsd-sysroot/aarch64)"
ARM64_CC := /usr/local/freebsd-sysroot/aarch64/bin/cc
else ifeq ($(GOARCH),arm64)
ARM64_CGO_LDFLAGS := "$$(pkg-config --libs libpcap) -libverbs"
ARM64_CGO_CFLAGS := "$$(pkg-config --cflags libpcap)"
ARM64_CC := ""
AMD64_CGO_LDFLAGS := "$$(pkg-config --libs libpcap --define-variable=prefix=/usr/local/freebsd-sysroot/amd64) -libverbs"
AMD64_CGO_CFLAGS := "$$(pkg-config --cflags libpcap --define-variable=prefix=/usr/local/freebsd-sysroot/amd64)"
AMD64_CC := /usr/local/freebsd-sysroot/amd64/bin/cc
endif

$(FREEBSD_AMD64_S_NAME): $(wildcard */*.go)
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=1 \
	CGO_LDFLAGS=$(AMD64_CGO_LDFLAGS) \
	CGO_CFLAGS=$(AMD64_CGO_CFLAGS) \
	CC=$(AMD64_CC) \
	go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
		-o $(FREEBSD_AMD64_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(FREEBSD_AMD64_S_NAME)"

$(FREEBSD_ARM64_S_NAME): $(wildcard */*.go)
	GOOS=freebsd GOARCH=arm64 CGO_ENABLED=1 \
	CGO_LDFLAGS=$(ARM64_CGO_LDFLAGS) \
	CGO_CFLAGS=$(ARM64_CGO_CFLAGS) \
	CC=$(ARM64_CC) \
	go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
		-o $(FREEBSD_ARM64_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(FREEBSD_ARM64_S_NAME)"


# armv7 is disabled because i can't consistently build with the same flags when building 
# on my ARM64 MAC vs. Ubuntu AMD64 image in Github... issue with -libverbs
$(FREEBSD_ARMV7_S_NAME): $(wildcard */*.go)
	GOOS=freebsd GOARCH=arm GOARM=7 CGO_ENABLED=1 \
	CGO_LDFLAGS="$$(pkg-config --libs libpcap --define-variable=prefix=/usr/local/freebsd-sysroot/arm7) -libverbs" \
	CGO_CFLAGS="$$(pkg-config --cflags libpcap --define-variable=prefix=/usr/local/freebsd-sysroot/arm7)" \
	CC=/usr/local/freebsd-sysroot/armv7/bin/cc \
	go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
		-o $(FREEBSD_ARMV7_S_NAME) ./cmd/udp-proxy-2020/...
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
	docker run -it --rm --entrypoint /bin/bash \
	    --volume $(shell pwd):/build/udp-proxy-2020 \
	    $(MIPS64_IMAGE)

.linux-mips64: $(LINUX_MIPS64_S_NAME)
$(LINUX_MIPS64_S_NAME): .prepare
	LDFLAGS='-l/usr/mips64-linux-gnuabi64/lib/libpcap.a' \
	    GOOS=linux GOARCH=mips64 CGO_ENABLED=1 CC=mips64-linux-gnuabi64-gcc \
	    PKG_CONFIG_PATH=/usr/mips64-linux-gnuabi64/lib/pkgconfig \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
	    	-o $(LINUX_MIPS64_S_NAME) ./cmd/udp-proxy-2020/...
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
	docker run -it --rm --entrypoint /bin/bash \
	    --volume $(shell pwd):/build/udp-proxy-2020 \
	    $(ARM_IMAGE)

#.linux-arm: $(LINUX_ARMV5_S_NAME) $(LINUX_ARMV6_S_NAME) $(LINUX_ARMV7_S_NAME) $(LINUX_ARM64_S_NAME)
.linux-arm: $(LINUX_ARM64_S_NAME) 
.linux-arm32: $(LINUX_ARMV7_S_NAME) $(LINUX_ARMV6_S_NAME) $(LINUX_ARMV5_S_NAME)
$(LINUX_ARMV5_S_NAME): .prepare
	LDFLAGS='-l/usr/arm-linux-gnueabi/lib/libpcap.a' \
		CFLAGS='-I/usr/arm-linux-gnueabi/include' \
		CC=arm-linux-gnueabi-gcc-11 \
		GOOS=linux GOARCH=arm GOARM=5 CGO_ENABLED=1 \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
			-o $(LINUX_ARMV5_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(LINUX_ARMV5_S_NAME)"

$(LINUX_ARMV6_S_NAME): .prepare
	LDFLAGS='-l/usr/arm-linux-gnueabi/lib/libpcap.a' \
		CFLAGS='-I/usr/arm-linux-gnueabi/include' \
		CC=arm-linux-gnueabi-gcc-11 \
		GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=1 \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
			-o $(LINUX_ARMV6_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(LINUX_ARMV6_S_NAME)"

$(LINUX_ARMV7_S_NAME): .prepare
	LDFLAGS='-l/usr/arm-linux-gnueabi/lib/libpcap.a' \
		CFLAGS='-I/usr/arm-linux-gnueabi/include' \
		CC=arm-linux-gnueabi-gcc-11 \
		GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
	    	-o $(LINUX_ARMV7_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(LINUX_ARMV7_S_NAME)"

$(LINUX_ARM64_S_NAME): .prepare
	LDFLAGS="-l/usr/aarch64-linux-gnu/lib/libpcap.a" \
	    GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
	    go build -ldflags '$(LDFLAGS) -linkmode external -extldflags -static' \
		-o $(LINUX_ARM64_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(LINUX_ARM64_S_NAME)"

######################################################################
# Targets for building macOS/Darwin (only valid on macOS)
######################################################################
ifeq ($(GOOS),darwin)
DARWIN_S_NAME := $(DIST_DIR)$(PROJECT_NAME)-$(PROJECT_VERSION)-darwin-$(GOARCH)
darwin: $(DARWIN_S_NAME) ## Build macOS/amd64 binary

$(DARWIN_S_NAME): ./cmd/udp-proxy-2020/*.go .prepare
	LDFLAGS="$$(pkg-config --libs libpcap)" \
	CFLAGS="$$(pkg-config --cflags libpcap)" \
	CGO_ENABLED=1 \
	go build -ldflags='$(LDFLAGS)' \
	     -o $(DARWIN_S_NAME) ./cmd/udp-proxy-2020/...
	@echo "Created: $(DARWIN_S_NAME)"
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
	go build -ldflags '$(LDFLAGS)' -o dist/udp-proxy-2020 ./cmd/udp-proxy-2020/...

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

package: .linux-amd64  ## Build deb/rpm packages
	docker build -t udp-proxy-2020-builder:latest -f Dockerfile.package .
	docker run --rm \
		-v $$(pwd)/dist:/root/dist \
		-e VERSION=$(PROJECT_VERSION) udp-proxy-2020-builder:latest

.PHONY: .print-freebsd-archs
.print-freebsd-archs:  ## Print the FreeBSD arches we are building for
	@echo $(FREEBSD_ARCHES)