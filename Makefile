GO=go
CI ?= false
GOTESTFLAGS ?= -race -timeout 120s
GOTESTFLAGSNORACE = -timeout 120s
GOFLAGS=-ldflags="-X unstable.build/go-tui/debug.Tag=$$(git describe --tags) -X unstable.build/go-tui/debug.Commit=$$(git rev-parse --short HEAD) -X unstable.build/go-tui/debug.Package=six"
UNAME := $(shell uname)

BIN=bin
TARGET=target
WASM_EXAMPLE_TARGET=bin/example_wasm
WASM_EXAMPLE=examples/wasm/main.go
WASM_EXAMPLE_STATIC=examples/wasm/static
LIBSRC=$(wildcard *.go) $(wildcard **/*.go) $(wildcard **/**/*.go) $(wildcard **/**/**/*.go)
EXAMPLESRC=$(filter-out $(WASM_EXAMPLE), $(wildcard examples/**/*.go))
EXAMPLEDIRS=$(sort $(dir $(EXAMPLESRC)))
EXAMPLES_NON_WASM=$(patsubst examples/%/,$(BIN)/example_%,$(EXAMPLEDIRS))
EXAMPLE_WASM_BLOB=$(WASM_EXAMPLE_TARGET)/main.wasm
EXAMPLES=$(EXAMPLES_NON_WASM)
EXECSRC=$(wildcard cmd/**/*.go) $(wildcard cmd/**/**/*.go)
EXECDIRS=$(sort $(dir $(EXECSRC)))
EXECS=$(patsubst cmd/%/,$(BIN)/%,$(EXECDIRS))
GOMOCKS=$(wildcard **/**/*_gomock.go) $(wildcard **/*_gomock.go)
RELEASE_FILES=$(wildcard release/*)

.PHONY: debug clean test coverage example_wasm generate sixdev \
	format docker-build-ci-gcp docker-push-ci-gcp cross-compile lint license assert_license

default: CGO_ENABLED=CGO_ENABLED=1
default: GOPRIVATE=github.com/unstablebuild,unstable.build/*
default: .git/hooks/pre-commit $(EXAMPLES) $(EXECS)

debug: GOFLAGS=-race
debug: CGO_ENABLED=CGO_ENABLED=1
debug: GOPRIVATE=github.com/unstablebuild,unstable.build/*
debug: $(EXAMPLES) $(EXECS)

sixdev: GOFLAGS=-race
sixdev: CGO_ENABLED=CGO_ENABLED=1
sixdev: $(EXAMPLES) bin/six

.git/hooks/pre-commit: .pre-commit-config.yaml
	@ pre-commit install

example_wasm: $(EXAMPLE_WASM_BLOB)

test: CI=$(CI)
test:
	@ go test ./.../... $(GOTESTFLAGS)

test: CI=$(CI)
test-no-race:
	@ go test ./.../... $(GOTESTFLAGSNORACE)

coverage: $(BIN)
	@ go test ./.../... -coverprofile $(BIN)/coverage
	@ go tool cover -html=$(BIN)/coverage

generate: GOPRIVATE=github.com/unstablebuild,unstable.build/*
generate:
	@ rm -rf **/*rpc*/*.pb.go
	@ go generate ./...

license:
	@ bluectl license LICENSE `find . -name \*.go | grep -v gomock | grep -v .pb.go | xargs`

assert_license:
	@ bluectl license -d LICENSE `find . -name \*.go | grep -v gomock | grep -v .pb.go | xargs`

format:
	@ go fmt ./.../...

cross-compile:
	@ . ./test_crosscompile.sh

lint:
	@ golangci-lint run --timeout=600s

clean:
	@rm -rf $(BIN) $(TARGET)

$(BIN):
	@mkdir $(BIN)

$(WASM_EXAMPLE_TARGET): $(BIN) $(WASM_EXAMPLE_STATIC)
	@rm -rf $(WASM_EXAMPLE_TARGET)
	@mkdir $(WASM_EXAMPLE_TARGET)
	@cp -R $(WASM_EXAMPLE_STATIC)/* $(WASM_EXAMPLE_TARGET)
	@cp `go env GOROOT`/misc/wasm/wasm_exec.js $(WASM_EXAMPLE_TARGET)/js

$(EXAMPLE_WASM_BLOB): $(WASM_EXAMPLE) $(LIBSRC) $(WASM_EXAMPLE_TARGET)
	$(CGO_ENABLED) GOOS=js GOARCH=wasm $(GO) build -o $(EXAMPLE_WASM_BLOB) $(WASM_EXAMPLE)

$(EXAMPLES_NON_WASM): $(EXAMPLESRC) $(LIBSRC) $(BIN)
	@cd $(patsubst bin/example_%,examples/%,$@) && $(CGO_ENABLED) $(GO) build $(GOFLAGS) -o ../../$@

$(EXECS): $(EXECSRC) $(LIBSRC) $(BIN)
	cd $(patsubst bin/%,cmd/%,$@) && $(CGO_ENABLED) $(GO) build $(GOFLAGS) -o ../../$@

make_release: CGO_ENABLED=CGO_ENABLED=1
make_release:
	@ mkdir -p $(TARGET)/$(TARGET_OS)_$(TARGET_ARCH)
	@ cp $(RELEASE_FILES) $(TARGET)
	@ $(CGO_ENABLED) GOARCH=$(TARGET_ARCH) $(TARGET_ARCH_FLAGS) GOOS=$(TARGET_OS) $(GO) build -o `pwd`/$(TARGET)/$(TARGET_OS)_$(TARGET_ARCH) $(GOFLAGS) ./cmd/...

ifeq ($(UNAME), Linux)
release: default
	@ rm -rf $(TARGET)
	@ TARGET_OS=linux TARGET_ARCH=amd64 $(MAKE) make_release
	@ cd $(TARGET) && tar -czvf six-release-`git describe --tags --dirty`.tar.gz *
endif
ifeq ($(UNAME), Darwin)
release: default
	@ rm -rf $(TARGET)
	@ TARGET_OS=darwin TARGET_ARCH=arm64 $(MAKE) make_release
	@ TARGET_OS=darwin TARGET_ARCH=amd64 $(MAKE) make_release
	@ cd $(TARGET) && tar -czvf six-release-`git describe --tags --dirty`.tar.gz *
endif

dist: release
	@ git fetch origin --tags
	@ ./dist.sh

docker-build-ci-gcp:
	@ docker buildx build -f Dockerfile.build --platform linux/amd64 -t us-central1-docker.pkg.dev/unstable-build-blue-dev/docker/go-tui-ci:latest --build-arg GIT_SSH_KEY="$$GIT_SSH_KEY" .

docker-push-ci-gcp:
	@ docker push us-central1-docker.pkg.dev/unstable-build-blue-dev/docker/go-tui-ci:latest
