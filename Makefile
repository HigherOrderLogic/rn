GO=go
CI ?= false
GOTESTFLAGS ?= -race -timeout 240s
GOTESTFLAGSNORACE = -timeout 240s
COMMON_LDFLAGS=-X unstable.build/go-tui/debug.Tag=$$(git describe --tags) -X unstable.build/go-tui/debug.Commit=$$(git rev-parse --short HEAD)
GOFLAGS=-ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=six"
RUNE_GOFLAGS=-tags=ebitensinglethread -ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=rune"
OXAPI_GOFLAGS=-ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=rune"
UNAME := $(shell uname)
VERSION=$(shell git describe --tags)
COMMIT=$(shell git rev-parse --short HEAD)
CODESIGN_IDENTITY ?= Developer ID Application: Unstable Build, LLC. (YYZRWD888J)
NOTARY_PROFILE ?= notary-profile

BIN=bin
TARGET=target
LIBSRC=$(wildcard *.go) $(wildcard **/*.go) $(wildcard **/**/*.go) $(wildcard **/**/**/*.go)
EXECSRC=$(wildcard cmd/**/*.go) $(wildcard cmd/**/**/*.go)
EXECMAIN=$(wildcard cmd/*/main.go)
EXECDIRS=$(sort $(dir $(EXECMAIN)))
EXECS=$(patsubst cmd/%/,$(BIN)/%,$(EXECDIRS))
CLAUDEIMPORT=$(BIN)/claudeimport
SPECIAL_EXECS=$(BIN)/rune $(BIN)/ox-api $(BIN)/rune-agent $(CLAUDEIMPORT)
GENERIC_EXECS=$(filter-out $(SPECIAL_EXECS),$(EXECS))
EXEC_PKGS=$(patsubst $(BIN)/%,./cmd/%,$(EXECS))
RELEASE_EXEC_PKGS=$(EXEC_PKGS)
GOMOCKS=$(wildcard **/**/*_gomock.go) $(wildcard **/*_gomock.go)
RELEASE_FILES=$(wildcard release/*)

.PHONY: debug clean test coverage generate sixdev rune rune-agent extension_go ox-api claudeimport \
	format docker-build-ci-gcp docker-push-ci-gcp cross-compile lint license assert_license \
	rune-release rune-release-amd64 rune-release-arm64 rune-make-release \
	rune-docker-build rune-docker-run rune-docker-build-gcp rune-docker-push-gcp \
	rune-docker-build-ci-gcp rune-docker-push-ci-gcp rune-app-amd64 rune-app-arm64 \
	rune-dmg rune-dmg-notarize rune-release-all \
	rune-agent-pkg rune-agent-sign rune-agent-notarize rune-agent-dist rune-agent-dist-notarized \
	extension-go-pkg extension-go-sign extension-go-notarize extension-go-dist extension-go-dist-notarized \
	runectl-pkg runectl-sign runectl-notarize runectl-dist runectl-dist-notarized \
	notary-credentials runectl

default: CGO_ENABLED=CGO_ENABLED=1
default: GOPRIVATE=github.com/unstablebuild,unstable.build/*
default: .git/hooks/pre-commit $(EXECS) $(CLAUDEIMPORT)

debug: GOFLAGS=-race
debug: CGO_ENABLED=CGO_ENABLED=1
debug: GOPRIVATE=github.com/unstablebuild,unstable.build/*
debug: $(EXECS) $(CLAUDEIMPORT)

sixdev: GOFLAGS=-race
sixdev: CGO_ENABLED=CGO_ENABLED=1
sixdev: bin/six

rune: CGO_ENABLED=CGO_ENABLED=1
rune: GOPRIVATE=github.com/unstablebuild,unstable.build/*
rune: $(BIN)/rune

rune-agent: CGO_ENABLED=CGO_ENABLED=1
rune-agent: GOPRIVATE=github.com/unstablebuild,unstable.build/*
rune-agent: $(BIN)/rune-agent

extension_go: CGO_ENABLED=CGO_ENABLED=1
extension_go: GOPRIVATE=github.com/unstablebuild,unstable.build/*
extension_go: $(BIN)/extension_go

ox-api: CGO_ENABLED=CGO_ENABLED=1
ox-api: GOPRIVATE=github.com/unstablebuild,unstable.build/*
ox-api: $(BIN)/ox-api

claudeimport: CGO_ENABLED=CGO_ENABLED=1
claudeimport: GOPRIVATE=github.com/unstablebuild,unstable.build/*
claudeimport: $(CLAUDEIMPORT)

.git/hooks/pre-commit: .pre-commit-config.yaml
	@ pre-commit install

test: CI=$(CI)
test:
	@ go test -vet=off ./.../... $(GOTESTFLAGS)

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
	@$(MAKE) -C cmd/rune-agent clean
	@$(MAKE) -C cmd/extension_go clean
	@$(MAKE) -C cmd/runectl clean
	@$(MAKE) -C cmd/rune clean

$(BIN):
	@mkdir $(BIN)

$(BIN)/rune: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/rune && $(CGO_ENABLED) $(GO) build $(RUNE_GOFLAGS) -o ../../$@

$(BIN)/ox-api: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/ox-api && $(CGO_ENABLED) $(GO) build $(OXAPI_GOFLAGS) -o ../../$@

$(BIN)/claudeimport: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/claudeimport && $(CGO_ENABLED) $(GO) build $(GOFLAGS) -o ../../$@

$(BIN)/rune-agent: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/rune-agent && $(CGO_ENABLED) $(GO) build $(GOFLAGS) -o ../../$@

$(GENERIC_EXECS): $(EXECSRC) $(LIBSRC) $(BIN)
	cd $(patsubst bin/%,cmd/%,$@) && $(CGO_ENABLED) $(GO) build $(GOFLAGS) -o ../../$@

$(BIN)/runectl: $(BIN)
	@GOBIN="`pwd`/$(BIN)" $(GO) install github.com/unstablebuild/rune-go-sdk/cmd/runectl

make_release: CGO_ENABLED=CGO_ENABLED=1
make_release:
	@ mkdir -p $(TARGET)/$(TARGET_OS)_$(TARGET_ARCH)
	@ cp $(RELEASE_FILES) $(TARGET)
	@ $(CGO_ENABLED) GOARCH=$(TARGET_ARCH) $(TARGET_ARCH_FLAGS) GOOS=$(TARGET_OS) $(GO) build -o `pwd`/$(TARGET)/$(TARGET_OS)_$(TARGET_ARCH) $(GOFLAGS) $(RELEASE_EXEC_PKGS)

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

rune-make-release:
	@$(MAKE) -C cmd/rune make-release TARGET_OS="$(TARGET_OS)" TARGET_ARCH="$(TARGET_ARCH)" TARGET_ARCH_FLAGS="$(TARGET_ARCH_FLAGS)"

rune-release:
	@$(MAKE) -C cmd/rune release

rune-release-amd64:
	@$(MAKE) -C cmd/rune release-amd64

rune-release-arm64:
	@$(MAKE) -C cmd/rune release-arm64

rune-docker-build:
	@$(MAKE) -C cmd/rune docker-build

rune-docker-run:
	@$(MAKE) -C cmd/rune docker-run

rune-docker-build-gcp:
	@$(MAKE) -C cmd/rune docker-build-gcp

rune-docker-push-gcp:
	@$(MAKE) -C cmd/rune docker-push-gcp

rune-docker-build-ci-gcp:
	@$(MAKE) -C cmd/rune docker-build-ci-gcp

rune-docker-push-ci-gcp:
	@$(MAKE) -C cmd/rune docker-push-ci-gcp

rune-app-amd64:
	@$(MAKE) -C cmd/rune app-amd64

rune-app-arm64:
	@$(MAKE) -C cmd/rune app-arm64

rune-dmg:
	@$(MAKE) -C cmd/rune dmg

rune-dmg-notarize:
	@$(MAKE) -C cmd/rune dmg-notarize

rune-release-all:
	@$(MAKE) -C cmd/rune release-all

runectl: CGO_ENABLED=CGO_ENABLED=1
runectl: $(BIN)/runectl

rune-agent-pkg:
	@$(MAKE) -C cmd/rune-agent pkg

rune-agent-sign:
	@$(MAKE) -C cmd/rune-agent sign

rune-agent-notarize:
	@$(MAKE) -C cmd/rune-agent notarize

rune-agent-dist:
	@$(MAKE) -C cmd/rune-agent dist

rune-agent-dist-notarized:
	@$(MAKE) -C cmd/rune-agent dist-notarized

extension-go-pkg:
	@$(MAKE) -C cmd/extension_go pkg

extension-go-sign:
	@$(MAKE) -C cmd/extension_go sign

extension-go-notarize:
	@$(MAKE) -C cmd/extension_go notarize

extension-go-dist:
	@$(MAKE) -C cmd/extension_go dist

extension-go-dist-notarized:
	@$(MAKE) -C cmd/extension_go dist-notarized

runectl-pkg:
	@$(MAKE) -C cmd/runectl pkg

runectl-sign:
	@$(MAKE) -C cmd/runectl sign

runectl-notarize:
	@$(MAKE) -C cmd/runectl notarize

runectl-dist:
	@$(MAKE) -C cmd/runectl dist

runectl-dist-notarized:
	@$(MAKE) -C cmd/runectl dist-notarized

notary-credentials:
	xcrun notarytool store-credentials "$(NOTARY_PROFILE)" --team-id "YYZRWD888J"
