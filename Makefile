GO=go
CI ?= false
GOTESTFLAGS ?= -race -timeout 240s
GOTESTFLAGSNORACE = -timeout 240s
COMMON_LDFLAGS=-X unstable.build/go-tui/debug.Tag=$$(git describe --tags) -X unstable.build/go-tui/debug.Commit=$$(git rev-parse --short HEAD)
GOFLAGS=-ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=six"
RUNE_GOFLAGS=-tags=ebitensinglethread -ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=rune"
OXAPI_GOFLAGS=
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
.PHONY: debug clean test coverage generate sixdev rune rune-agent ox-api claudeimport \
	format docker-build-ci-gcp docker-push-ci-gcp cross-compile lint license assert_license dist \
	rune-release rune-release-amd64 rune-release-arm64 rune-make-release \
	rune-app-delve \
	rune-docker-build rune-docker-run \
	ox-api-docker-build-gcp ox-api-docker-push-gcp \
	ox-api-docker-build-gcp-prod ox-api-docker-push-gcp-prod \
	rune-linux-cross-compile rune-app-amd64 rune-app-arm64 \
	rune-dmg rune-dmg-amd64 rune-dmg-notarize rune-dmg-amd64-notarize rune-release-all \
	rune-agent-pkg rune-agent-sign rune-agent-notarize \
	rune-agent-prod-dist rune-agent-staging-dist \
	rune-agent-prod-dist-notarized rune-agent-staging-dist-notarized \
	rune-agent-linux-cross-compile \
	rune-agent-release-linux-amd64 rune-agent-release-linux-arm64 \
	rune-agent-prod-dist-linux-amd64 rune-agent-staging-dist-linux-amd64 \
	rune-agent-prod-dist-linux-arm64 rune-agent-staging-dist-linux-arm64 \
	fuzzy-search fuzzy-search-pkg \
	fuzzy-search-prod-dist fuzzy-search-staging-dist \
	runectl-pkg runectl-sign runectl-notarize \
	runectl-prod-dist runectl-staging-dist \
	runectl-prod-dist-notarized runectl-staging-dist-notarized \
	notary-credentials runectl \
	rune-release-linux-amd64 rune-release-linux-arm64 \
	rune-prod-dist-linux-amd64 rune-prod-dist-linux-arm64 \
	rune-prod-dist-darwin-arm64 rune-prod-dist-darwin-amd64 \
	rune-staging-dist-linux-amd64 rune-staging-dist-linux-arm64 \
	rune-staging-dist-darwin-arm64 rune-staging-dist-darwin-amd64 \
	deps llamacpp-libs llamacpp-init \
	ox-api-init \
	fuzz fuzz-list \
	manual-ssh-test \
	$(filter workspace/workspacessh/manual_test/%.sh,$(MAKECMDGOALS))

LLAMACPP_STAMP=$(TARGET)/llamacpp-libs.stamp

# bluectl config matrix. Each leaf config pins BOTH auth.project-id and
# release.collection so the publishing env + destination bucket are
# selected by the make target rather than by ~/.bluectl/config or
# whatever bucket the user's local config happens to name.
#
# The host targets (rune-agent-{prod,staging}-dist, runectl-*, fuzzy-*)
# derive their os/arch from the host running make.
BLUECTL_HOST_OS   := $(shell uname | awk '{print tolower($$0)}')
BLUECTL_HOST_ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
BLUECTL_CONFIG_ROOT := $(abspath deploy/bluectl)

# Helper: resolve a leaf config dir given env (prod|staging) and os-arch slug.
BLUECTL_CONFIG = $(BLUECTL_CONFIG_ROOT)/$(1)/$(2)

default: CGO_ENABLED=CGO_ENABLED=1
default: GOPRIVATE=github.com/unstablebuild,unstable.build/*
default: .git/hooks/pre-commit deps $(EXECS) $(CLAUDEIMPORT)

debug: GOFLAGS=-race
debug: CGO_ENABLED=CGO_ENABLED=1
debug: GOPRIVATE=github.com/unstablebuild,unstable.build/*
debug: deps $(EXECS) $(CLAUDEIMPORT)

sixdev: GOFLAGS=-race
sixdev: CGO_ENABLED=CGO_ENABLED=1
sixdev: bin/six

rune: CGO_ENABLED=CGO_ENABLED=1
rune: GOPRIVATE=github.com/unstablebuild,unstable.build/*
rune: $(BIN)/rune

rune-agent: CGO_ENABLED=CGO_ENABLED=1
rune-agent: GOPRIVATE=github.com/unstablebuild,unstable.build/*
rune-agent: $(BIN)/rune-agent

ox-api: CGO_ENABLED=CGO_ENABLED=1
ox-api: GOPRIVATE=github.com/unstablebuild,unstable.build/*
ox-api: ox-api-init $(BIN)/ox-api

claudeimport: CGO_ENABLED=CGO_ENABLED=1
claudeimport: GOPRIVATE=github.com/unstablebuild,unstable.build/*
claudeimport: $(CLAUDEIMPORT)

.git/hooks/pre-commit: .pre-commit-config.yaml
	@ pre-commit install

test: CI=$(CI)
test: $(LLAMACPP_STAMP)
	@ go test -vet=off ./.../... $(GOTESTFLAGS)

test: CI=$(CI)
test-no-race: $(LLAMACPP_STAMP)
	@ go test ./.../... $(GOTESTFLAGSNORACE)

coverage: $(BIN)
	@ go test ./.../... -coverprofile $(BIN)/coverage
	@ go tool cover -html=$(BIN)/coverage

# fuzz runs every `Fuzz*` target in the repository for FUZZTIME each.
# Each target runs sequentially with -parallel=1 because some fuzzers
# (notably the llamacpp model-backed ones) load multi-GiB GGUFs into
# Metal and cannot be safely run in parallel worker processes.
#
# Override defaults on the command line:
#   make fuzz FUZZTIME=30s         # longer per-target budget
#   make fuzz FUZZ_PKG=./component/markdown/...
FUZZTIME ?= 10s
FUZZ_PKG ?= ./...
FUZZ_TEST_FLAGS ?= -race -parallel=1 -count=1

fuzz: $(LLAMACPP_STAMP)
	@ set -e; \
	pkgs=$$(go list -f '{{if (or .TestGoFiles .XTestGoFiles)}}{{.ImportPath}}{{end}}' $(FUZZ_PKG)); \
	for pkg in $$pkgs; do \
		targets=$$(go test -list '^Fuzz' $$pkg 2>/dev/null | grep '^Fuzz' || true); \
		[ -z "$$targets" ] && continue; \
		for t in $$targets; do \
			echo "==> $$pkg $$t (-fuzztime=$(FUZZTIME))"; \
			go test $(FUZZ_TEST_FLAGS) -run='^$$' -fuzz='^'"$$t"'$$' -fuzztime=$(FUZZTIME) $$pkg || exit $$?; \
		done; \
	done

# fuzz-list prints every fuzz target the repository ships, grouped by
# package. Useful when you want to invoke `go test -fuzz=…` directly.
fuzz-list:
	@ set -e; \
	pkgs=$$(go list -f '{{if (or .TestGoFiles .XTestGoFiles)}}{{.ImportPath}}{{end}}' ./...); \
	for pkg in $$pkgs; do \
		targets=$$(go test -list '^Fuzz' $$pkg 2>/dev/null | grep '^Fuzz' || true); \
		[ -z "$$targets" ] && continue; \
		echo "$$pkg:"; \
		for t in $$targets; do echo "  $$t"; done; \
	done

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
	@$(MAKE) -C cmd/extension_fuzzy_search clean
	@$(MAKE) -C cmd/runectl clean
	@$(MAKE) -C cmd/rune clean

$(BIN):
	@mkdir $(BIN)

$(BIN)/rune: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/rune && $(CGO_ENABLED) $(GO) build $(RUNE_GOFLAGS) -o ../../$@

$(BIN)/ox-api: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/ox-api && $(CGO_ENABLED) $(GO) build $(OXAPI_GOFLAGS) -o ../../$@ .

$(BIN)/claudeimport: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/claudeimport && $(CGO_ENABLED) $(GO) build $(GOFLAGS) -o ../../$@

$(BIN)/rune-agent: $(EXECSRC) $(LIBSRC) $(BIN) $(LLAMACPP_STAMP)
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

dist: clean release
	@ git fetch origin --tags
	@ ./dist.sh

docker-build-ci-gcp:
	@ docker buildx build -f deploy/build/Dockerfile --platform linux/amd64 -t us-central1-docker.pkg.dev/unstable-build-blue-dev/docker/go-tui-ci:latest --build-arg GIT_SSH_KEY="$$GIT_SSH_KEY" .

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

ox-api-docker-build-gcp:
	@$(MAKE) -C cmd/ox-api docker-build-gcp

ox-api-docker-push-gcp:
	@$(MAKE) -C cmd/ox-api docker-push-gcp

ox-api-docker-build-gcp-prod:
	@$(MAKE) -C cmd/ox-api docker-build-gcp-prod

ox-api-docker-push-gcp-prod:
	@$(MAKE) -C cmd/ox-api docker-push-gcp-prod

rune-linux-cross-compile:
	@$(MAKE) -C cmd/rune linux-cross-compile

rune-release-linux-amd64:
	@$(MAKE) -C cmd/rune release-linux-amd64

rune-release-linux-arm64:
	@$(MAKE) -C cmd/rune release-linux-arm64

# rune-prod-dist-* / rune-staging-dist-*: build a release artifact and
# upload it to the corresponding public download bucket.
#
#   prod    -> gs://downloads.rune.build       (api.rune.build / rpc.rune.build:443)
#   staging -> gs://downloads.unstable.build   (api.unstable.build / rpc.unstable.build:443)
rune-prod-dist-linux-amd64: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-amd64

rune-prod-dist-linux-arm64: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-arm64

rune-prod-dist-darwin-arm64: clean
	@$(MAKE) -C cmd/rune prod-dist-darwin-arm64

rune-prod-dist-darwin-amd64: clean
	@$(MAKE) -C cmd/rune prod-dist-darwin-amd64

rune-staging-dist-linux-amd64: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-amd64

rune-staging-dist-linux-arm64: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-arm64

rune-staging-dist-darwin-arm64: clean
	@$(MAKE) -C cmd/rune staging-dist-darwin-arm64

rune-staging-dist-darwin-amd64: clean
	@$(MAKE) -C cmd/rune staging-dist-darwin-amd64

# manual-ssh-test runs a named manual SSH scenario script using the
# real Linux release rune binary as the remote workspace server.
#
# We pick the linux arch that matches the docker host's native
# architecture so the openssh test container runs the rune binary
# without QEMU emulation (the harness's e2e tests do the same; see
# containerGOARCH() in workspace/workspacessh/test/harness.go).
#
# Usage:
#   make manual-ssh-test workspace/workspacessh/manual_test/01_host_key_match.sh
RUNE_LINUX_HOST_ARCH := $(shell uname -m | sed -e 's/^arm64$$/arm64/' -e 's/^aarch64$$/arm64/' -e 's/^x86_64$$/amd64/' -e 's/^amd64$$/amd64/')
RUNE_LINUX_REMOTE_BIN := $(TARGET)/rune_linux_$(RUNE_LINUX_HOST_ARCH)/rune.app/bin/rune
MANUAL_SSH_SCRIPT := $(filter workspace/workspacessh/manual_test/%.sh,$(MAKECMDGOALS))

manual-ssh-test: rune-release-linux-$(RUNE_LINUX_HOST_ARCH)
	@if [ -z "$(MANUAL_SSH_SCRIPT)" ]; then \
		echo "usage: make manual-ssh-test workspace/workspacessh/manual_test/<file>.sh" >&2; \
		exit 2; \
	fi
	@RUNE_REMOTE_BIN=$(RUNE_LINUX_REMOTE_BIN) ./$(MANUAL_SSH_SCRIPT)

# Swallow the script path argument so make doesn't try to (re)build
# the .sh file as a target. The actual script is invoked by the
# manual-ssh-test recipe above.
workspace/workspacessh/manual_test/%.sh:
	@:

rune-app-amd64:
	@$(MAKE) -C cmd/rune app-amd64

rune-app-arm64:
	@$(MAKE) -C cmd/rune app-arm64

rune-app-delve:
	@$(MAKE) -C cmd/rune app-delve

rune-dmg:
	@$(MAKE) -C cmd/rune dmg

rune-dmg-amd64:
	@$(MAKE) -C cmd/rune dmg-amd64

rune-dmg-notarize:
	@$(MAKE) -C cmd/rune dmg-notarize

rune-dmg-amd64-notarize:
	@$(MAKE) -C cmd/rune dmg-amd64-notarize

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

rune-agent-prod-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/rune-agent dist

rune-agent-staging-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/rune-agent dist

rune-agent-prod-dist-notarized: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/rune-agent dist-notarized

rune-agent-staging-dist-notarized: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/rune-agent dist-notarized

rune-agent-linux-cross-compile:
	@$(MAKE) -C cmd/rune-agent linux-cross-compile

rune-agent-release-linux-amd64:
	@$(MAKE) -C cmd/rune-agent release-linux-amd64

rune-agent-release-linux-arm64:
	@$(MAKE) -C cmd/rune-agent release-linux-arm64

rune-agent-prod-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-amd64) $(MAKE) -C cmd/rune-agent dist-linux-amd64

rune-agent-staging-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-amd64) $(MAKE) -C cmd/rune-agent dist-linux-amd64

rune-agent-prod-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-arm64) $(MAKE) -C cmd/rune-agent dist-linux-arm64

rune-agent-staging-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-arm64) $(MAKE) -C cmd/rune-agent dist-linux-arm64

fuzzy-search: CGO_ENABLED=CGO_ENABLED=1
fuzzy-search: $(BIN)/extension_fuzzy_search

fuzzy-search-pkg:
	@$(MAKE) -C cmd/extension_fuzzy_search pkg

fuzzy-search-prod-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/extension_fuzzy_search dist

fuzzy-search-staging-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/extension_fuzzy_search dist

runectl-pkg:
	@$(MAKE) -C cmd/runectl pkg

runectl-sign:
	@$(MAKE) -C cmd/runectl sign

runectl-notarize:
	@$(MAKE) -C cmd/runectl notarize

runectl-prod-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/runectl dist

runectl-staging-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/runectl dist

runectl-prod-dist-notarized: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/runectl dist-notarized

runectl-staging-dist-notarized: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/runectl dist-notarized

notary-credentials:
	xcrun notarytool store-credentials "$(NOTARY_PROFILE)" --team-id "YYZRWD888J"

# deps brings in git-managed prerequisites that are needed for local builds.
deps: llamacpp-init ox-api-init

# llamacpp-init makes sure the llama.cpp git submodule is checked out. It is
# safe to run repeatedly; the submodule Makefile is also defensive about
# running on a populated tree.
llamacpp-init:
	@ git submodule update --init --recursive cmd/rune-agent/llm/llamacpp/llama.cpp

# ox-api-init makes sure the ox-api git submodule is checked out so the
# cmd/ox-api package compiles. Safe to run repeatedly.
ox-api-init:
	@ git submodule update --init --recursive cmd/ox-api

# llamacpp-libs builds the static libraries that the llamacpp cgo bindings
# link against. Skipped silently when the libs are already present and
# fresher than the submodule's CMakeLists.txt — the submodule Makefile
# handles its own up-to-date checks.

$(LLAMACPP_STAMP): deps
	@ mkdir -p $(dir $@)
	@ $(MAKE) -C cmd/rune-agent/llm/llamacpp libs
	@ touch $@

llamacpp-libs: $(LLAMACPP_STAMP)
