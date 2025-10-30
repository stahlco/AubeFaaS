PROJECT_NAME := "AubeFaaS"
GO_FILES := $(shell find . -name '*.go' | grep -v /vendor/ | grep -v /ext/ | grep -v _test.go)
# TEST_DIR := ./test
PKG := "github.com/stahlco/${PROJECT_NAME}"

SUPPORTED_ARCH=arm64
RUNTIME := $(shell find pkg/docker/runtimes -name Dockerfile | xargs -n1 dirname | xargs -n1 basename)

OS=$(shell go env GOOS)
ARCH=$(shell go env GOARCH)

.PHONY: build
build: aubefaas-${OS}-${ARCH}

.PHONY: start
start: aubefaas-${OS}-${ARCH}
	./$<

.PHONY: clean


# embeds the FS for each runtime (only python), based on the given architecture (arm64)
define arch_build
pkg/docker/runtimes-$(arch): $(foreach runtime, $(RUNTIMES), pkg/docker/runtimes-$(arch)/$(runtime))
endef
$(foreach arch, $(SUPPORTED_ARCH), $(eval $(arch_build)))

define runtime_build
.PHONY: pkg/docker/runtimes-$(arch)/$(runtime)
pkg/docker/runtimes-$(arch)/$(runtime): pkg/docker/runtimes-$(arch)/$(runtime)/Dockerfile

pkg/docker/runtimes-$(arch)/$(runtime)/Dockerfile: pkg/docker/runtimes/$(runtime)/Dockerfile
	mkdir -p $$(@D)
	cp -r pkg/docker/runtimes/$(runtime)/Dockerfile $$@
endef
$(foreach arch, $(SUPPORTED_ARCH), $(foreach runtime, $(RUNTIME), $(eval $(runtime_build))))

cmd/controlplane/rproxy-%.bin: $(GO_FILES)
	GOOS=$(word 1,$(subst -, ,$*)) GOARCH=$(word 2,$(subst -, ,$*)) go build -o $@ -v ./cmd/rproxy

# Only for darwin for now
aubefaas-darwin-%: cmd/controlplane/rproxy-darwin-%.bin pkg/docker/runtimes-% $(GO_FILES)
	GOOS=darwin GOARCH=$* go build -o $@ -v ./cmd/controlplane

aubefaas-linux-%: cmd/manager/rproxy-linux-%.bin pkg/docker/runtimes-% $(GO_FILES)
	GOOS=linux GOARCH=$* go build -o $@ -v ./cmd/controlplane