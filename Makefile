GO=go
CI ?= false
GOTESTFLAGS ?= -race -timeout 240s
GOTESTFLAGSNORACE = -timeout 240s
# RUNE_DEBUG_BUILD, when set to "true", flips an in-binary feature
# flag that exposes debug-only ex commands such as :panic and :crash.
# Defaults to off; the `debug` target sets it via a target-specific
# variable. Recursive (=) so target-specific overrides propagate to
# prerequisites that re-expand COMMON_LDFLAGS.
RUNE_DEBUG_BUILD ?=
DEBUG_LDFLAGS=$(if $(filter true,$(RUNE_DEBUG_BUILD)),-X unstable.build/go-tui/debug.DebugBuild=true)
# RACE_FLAG mirrors RUNE_DEBUG_BUILD so the race detector is enabled
# for every binary the `debug` target produces, including the rune
# binary (which builds with RUNE_GOFLAGS rather than GOFLAGS). This is
# what makes :datarace actually crash the debug build.
RACE_FLAG=$(if $(filter true,$(RUNE_DEBUG_BUILD)),-race)
COMMON_LDFLAGS=-X unstable.build/go-tui/debug.Tag=$$(git describe --tags) -X unstable.build/go-tui/debug.Commit=$$(git rev-parse --short HEAD) $(DEBUG_LDFLAGS)
GOFLAGS=$(RACE_FLAG) -ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=six"
RUNE_GOFLAGS=$(RACE_FLAG) -tags=ebitensinglethread -ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=rune"
OXAPI_GOFLAGS=-ldflags="-X main.Tag=$$(git describe --tags --always --dirty) -X main.Commit=$$(git rev-parse --short HEAD)$$(git diff --quiet || echo -dirty)"
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
.PHONY: debug clean purge test coverage generate rune rune-agent ox-api claudeimport \
	format docker-build-ci-gcp docker-push-ci-gcp cross-compile lint license assert_license dist \
	rune-release rune-release-amd64 rune-release-arm64 rune-make-release \
	rune-app-delve \
	rune-docker-build rune-docker-run \
	ox-api-docker-build-gcp ox-api-docker-push-gcp-staging \
	ox-api-docker-build-gcp-prod ox-api-docker-push-gcp-prod \
	rune-linux-cross-compile rune-app-amd64 rune-app-arm64 \
	rune-prod-app-arm64 \
	rune-dmg rune-dmg-amd64 rune-dmg-notarize rune-dmg-amd64-notarize rune-release-all \
	rune-agent-pkg rune-agent-sign rune-agent-notarize \
	rune-agent-prod-dist rune-agent-staging-dist \
	rune-agent-prod-dist-notarized rune-agent-staging-dist-notarized \
	rune-agent-linux-cross-compile \
	rune-agent-release-linux-amd64 rune-agent-release-linux-arm64 \
	rune-agent-release-linux-amd64-cross rune-agent-release-linux-arm64-cross \
	rune-agent-prod-dist-linux-amd64 rune-agent-staging-dist-linux-amd64 \
	rune-agent-prod-dist-linux-arm64 rune-agent-staging-dist-linux-arm64 \
	rune-agent-prod-dist-linux-amd64-cross rune-agent-staging-dist-linux-amd64-cross \
	rune-agent-prod-dist-linux-arm64-cross rune-agent-staging-dist-linux-arm64-cross \
	rune-agent-prod-dist-darwin-amd64 rune-agent-staging-dist-darwin-amd64 \
	rune-agent-prod-dist-darwin-arm64 rune-agent-staging-dist-darwin-arm64 \
	fuzzy-search fuzzy-search-pkg \
	fuzzy-search-prod-dist fuzzy-search-staging-dist \
	fuzzy-search-linux-cross-compile \
	fuzzy-search-release-linux-amd64 fuzzy-search-release-linux-arm64 \
	fuzzy-search-release-linux-amd64-cross fuzzy-search-release-linux-arm64-cross \
	fuzzy-search-prod-dist-linux-amd64 fuzzy-search-staging-dist-linux-amd64 \
	fuzzy-search-prod-dist-linux-arm64 fuzzy-search-staging-dist-linux-arm64 \
	fuzzy-search-prod-dist-linux-amd64-cross fuzzy-search-staging-dist-linux-amd64-cross \
	fuzzy-search-prod-dist-linux-arm64-cross fuzzy-search-staging-dist-linux-arm64-cross \
	fuzzy-search-prod-dist-darwin-amd64 fuzzy-search-staging-dist-darwin-amd64 \
	fuzzy-search-prod-dist-darwin-arm64 fuzzy-search-staging-dist-darwin-arm64 \
	runectl-pkg runectl-sign runectl-notarize \
	runectl-prod-dist runectl-staging-dist \
	runectl-prod-dist-notarized runectl-staging-dist-notarized \
	runectl-prod-dist-linux-amd64 runectl-staging-dist-linux-amd64 \
	runectl-prod-dist-linux-arm64 runectl-staging-dist-linux-arm64 \
	runectl-prod-dist-darwin-amd64 runectl-staging-dist-darwin-amd64 \
	runectl-prod-dist-darwin-arm64 runectl-staging-dist-darwin-arm64 \
	notary-credentials runectl \
	rune-release-linux-amd64 rune-release-linux-arm64 \
	rune-release-linux-amd64-native rune-release-linux-arm64-native \
	rune-release-linux-amd64-cross rune-release-linux-arm64-cross \
	rune-prod-dist-linux-amd64 rune-prod-dist-linux-arm64 \
	rune-prod-dist-linux-amd64-native rune-prod-dist-linux-arm64-native \
	rune-prod-dist-linux-amd64-cross rune-prod-dist-linux-arm64-cross \
	rune-prod-dist-darwin-arm64 rune-prod-dist-darwin-amd64 \
	rune-staging-dist-linux-amd64 rune-staging-dist-linux-arm64 \
	rune-staging-dist-linux-amd64-native rune-staging-dist-linux-arm64-native \
	rune-staging-dist-linux-amd64-cross rune-staging-dist-linux-arm64-cross \
	rune-staging-dist-darwin-arm64 rune-staging-dist-darwin-amd64 \
	deps rune-llamacpp-libs rune-llamacpp-init \
	ox-api-init docs-init \
	fuzz fuzz-list \
	manual-ssh-test \
	dist-tar-with-src dist-dmg-with-src dist-min-macos \
	$(filter workspace/workspacessh/manual_test/%.sh,$(MAKECMDGOALS))

RUNE_LLAMACPP_STAMP=$(TARGET)/rune-llamacpp-libs.stamp

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

debug: RUNE_DEBUG_BUILD := true
debug: CGO_ENABLED=CGO_ENABLED=1
debug: GOPRIVATE=github.com/unstablebuild,unstable.build/*
debug: deps $(EXECS) $(CLAUDEIMPORT)

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
test: docs-init $(RUNE_LLAMACPP_STAMP)
	@ go test -vet=off ./.../... $(GOTESTFLAGS)

test: CI=$(CI)
test-no-race: docs-init $(RUNE_LLAMACPP_STAMP)
	@ go test ./.../... $(GOTESTFLAGSNORACE)

coverage: docs-init $(BIN)
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

fuzz: docs-init $(RUNE_LLAMACPP_STAMP)
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
generate: docs-init
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

lint: docs-init
	@ golangci-lint run --timeout=600s

clean:
	@rm -rf $(BIN) $(TARGET)
	@$(MAKE) -C cmd/rune-agent clean
	@$(MAKE) -C cmd/extension_fuzzy_search clean
	@$(MAKE) -C cmd/runectl clean
	@$(MAKE) -C cmd/rune clean

# purge does everything clean does and additionally wipes the llama.cpp build
# tree (cmake _build with its .o objects) and the copied static libs, forcing
# a full rebuild of the local-model backend on the next build.
purge: clean
	@$(MAKE) -C llm/llamacpp clean
	@rm -f $(RUNE_LLAMACPP_STAMP)

$(BIN):
	@mkdir $(BIN)

$(BIN)/rune: $(EXECSRC) $(LIBSRC) $(BIN) docs-init $(RUNE_LLAMACPP_STAMP)
	@cd cmd/rune && $(CGO_ENABLED) $(GO) build $(RUNE_GOFLAGS) -o ../../$@

$(BIN)/ox-api: $(EXECSRC) $(LIBSRC) $(BIN)
	@cd cmd/ox-api && $(CGO_ENABLED) $(GO) build $(OXAPI_GOFLAGS) -o ../../$@ .

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

ox-api-docker-push-gcp-staging:
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

rune-release-linux-amd64-native:
	@$(MAKE) -C cmd/rune release-linux-amd64-native

rune-release-linux-arm64-native:
	@$(MAKE) -C cmd/rune release-linux-arm64-native

rune-release-linux-amd64-cross:
	@$(MAKE) -C cmd/rune release-linux-amd64-cross

rune-release-linux-arm64-cross:
	@$(MAKE) -C cmd/rune release-linux-arm64-cross

# rune-prod-dist-* / rune-staging-dist-*: build a release artifact and
# upload it to the corresponding public download bucket.
#
#   prod    -> gs://downloads.rune.build       (api.rune.build / rpc.rune.build:443)
#   staging -> gs://downloads.unstable.build   (api.unstable.build / rpc.unstable.build:443)
rune-prod-dist-linux-amd64: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-amd64

rune-prod-dist-linux-arm64: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-arm64

rune-prod-dist-linux-amd64-native: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-amd64-native

rune-prod-dist-linux-arm64-native: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-arm64-native

rune-prod-dist-linux-amd64-cross: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-amd64-cross

rune-prod-dist-linux-arm64-cross: clean
	@$(MAKE) -C cmd/rune prod-dist-linux-arm64-cross

rune-prod-dist-darwin-arm64: clean
	@$(MAKE) -C cmd/rune prod-dist-darwin-arm64

rune-prod-dist-darwin-amd64: clean
	@$(MAKE) -C cmd/rune prod-dist-darwin-amd64

rune-staging-dist-linux-amd64: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-amd64

rune-staging-dist-linux-arm64: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-arm64

rune-staging-dist-linux-amd64-native: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-amd64-native

rune-staging-dist-linux-arm64-native: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-arm64-native

rune-staging-dist-linux-amd64-cross: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-amd64-cross

rune-staging-dist-linux-arm64-cross: clean
	@$(MAKE) -C cmd/rune staging-dist-linux-arm64-cross

rune-staging-dist-darwin-arm64: clean
	@$(MAKE) -C cmd/rune staging-dist-darwin-arm64

rune-staging-dist-darwin-amd64: clean
	@$(MAKE) -C cmd/rune staging-dist-darwin-amd64

# dist-tar-with-src / dist-dmg-with-src exercise the .go-source publish
# gate (cmd/verify-no-go-source.sh) by driving every component's dist.sh
# against a stub artifact that intentionally embeds a .go file and
# asserting each script aborts before publishing. dist-dmg-with-src is a
# no-op skip off macOS (needs hdiutil).
dist-tar-with-src:
	@./cmd/dist-with-src-test.sh tar

dist-dmg-with-src:
	@./cmd/dist-with-src-test.sh dmg

# dist-min-macos exercises the minimum-macOS publish gate
# (cmd/verify-min-macos.sh + cmd/rune/dist.sh): it builds stub artifacts
# with a known Mach-O minos and asserts the gate refuses to publish one
# whose floor exceeds what we advertise, and fails closed when the floor
# is unset. macOS only (needs clang + otool); a no-op skip elsewhere.
dist-min-macos:
	@./cmd/dist-min-macos-test.sh

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

# Like rune-app-arm64 but bakes the prod API endpoints into the binary.
rune-prod-app-arm64:
	@$(MAKE) -C cmd/rune app-arm64 RUNE_ENV=prod

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

rune-agent-release-linux-amd64-cross:
	@$(MAKE) -C cmd/rune-agent release-linux-amd64-cross

rune-agent-release-linux-arm64-cross:
	@$(MAKE) -C cmd/rune-agent release-linux-arm64-cross

rune-agent-prod-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-amd64) $(MAKE) -C cmd/rune-agent dist-linux-amd64

rune-agent-staging-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-amd64) $(MAKE) -C cmd/rune-agent dist-linux-amd64

rune-agent-prod-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-arm64) $(MAKE) -C cmd/rune-agent dist-linux-arm64

rune-agent-staging-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-arm64) $(MAKE) -C cmd/rune-agent dist-linux-arm64

rune-agent-prod-dist-linux-amd64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-amd64) $(MAKE) -C cmd/rune-agent dist-linux-amd64-cross

rune-agent-staging-dist-linux-amd64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-amd64) $(MAKE) -C cmd/rune-agent dist-linux-amd64-cross

rune-agent-prod-dist-linux-arm64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-arm64) $(MAKE) -C cmd/rune-agent dist-linux-arm64-cross

rune-agent-staging-dist-linux-arm64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-arm64) $(MAKE) -C cmd/rune-agent dist-linux-arm64-cross

rune-agent-prod-dist-darwin-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,darwin-amd64) $(MAKE) -C cmd/rune-agent dist-darwin-amd64

rune-agent-staging-dist-darwin-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,darwin-amd64) $(MAKE) -C cmd/rune-agent dist-darwin-amd64

rune-agent-prod-dist-darwin-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,darwin-arm64) $(MAKE) -C cmd/rune-agent dist-darwin-arm64

rune-agent-staging-dist-darwin-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,darwin-arm64) $(MAKE) -C cmd/rune-agent dist-darwin-arm64

fuzzy-search: CGO_ENABLED=CGO_ENABLED=1
fuzzy-search: $(BIN)/extension_fuzzy_search

fuzzy-search-pkg:
	@$(MAKE) -C cmd/extension_fuzzy_search pkg

fuzzy-search-prod-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/extension_fuzzy_search dist

fuzzy-search-staging-dist: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,$(BLUECTL_HOST_OS)-$(BLUECTL_HOST_ARCH)) $(MAKE) -C cmd/extension_fuzzy_search dist

fuzzy-search-linux-cross-compile:
	@$(MAKE) -C cmd/extension_fuzzy_search linux-cross-compile

fuzzy-search-release-linux-amd64:
	@$(MAKE) -C cmd/extension_fuzzy_search release-linux-amd64

fuzzy-search-release-linux-arm64:
	@$(MAKE) -C cmd/extension_fuzzy_search release-linux-arm64

fuzzy-search-release-linux-amd64-cross:
	@$(MAKE) -C cmd/extension_fuzzy_search release-linux-amd64-cross

fuzzy-search-release-linux-arm64-cross:
	@$(MAKE) -C cmd/extension_fuzzy_search release-linux-arm64-cross

fuzzy-search-prod-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-amd64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-amd64

fuzzy-search-staging-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-amd64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-amd64

fuzzy-search-prod-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-arm64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-arm64

fuzzy-search-staging-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-arm64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-arm64

fuzzy-search-prod-dist-linux-amd64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-amd64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-amd64-cross

fuzzy-search-staging-dist-linux-amd64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-amd64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-amd64-cross

fuzzy-search-prod-dist-linux-arm64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-arm64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-arm64-cross

fuzzy-search-staging-dist-linux-arm64-cross: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-arm64) $(MAKE) -C cmd/extension_fuzzy_search dist-linux-arm64-cross

fuzzy-search-prod-dist-darwin-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,darwin-amd64) $(MAKE) -C cmd/extension_fuzzy_search dist-darwin-amd64

fuzzy-search-staging-dist-darwin-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,darwin-amd64) $(MAKE) -C cmd/extension_fuzzy_search dist-darwin-amd64

fuzzy-search-prod-dist-darwin-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,darwin-arm64) $(MAKE) -C cmd/extension_fuzzy_search dist-darwin-arm64

fuzzy-search-staging-dist-darwin-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,darwin-arm64) $(MAKE) -C cmd/extension_fuzzy_search dist-darwin-arm64

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

runectl-prod-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-amd64) $(MAKE) -C cmd/runectl dist-linux-amd64

runectl-staging-dist-linux-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-amd64) $(MAKE) -C cmd/runectl dist-linux-amd64

runectl-prod-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,linux-arm64) $(MAKE) -C cmd/runectl dist-linux-arm64

runectl-staging-dist-linux-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,linux-arm64) $(MAKE) -C cmd/runectl dist-linux-arm64

runectl-prod-dist-darwin-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,darwin-amd64) $(MAKE) -C cmd/runectl dist-darwin-amd64

runectl-staging-dist-darwin-amd64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,darwin-amd64) $(MAKE) -C cmd/runectl dist-darwin-amd64

runectl-prod-dist-darwin-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,prod,darwin-arm64) $(MAKE) -C cmd/runectl dist-darwin-arm64

runectl-staging-dist-darwin-arm64: clean
	@BLUECTL_CONFIG_DIR=$(call BLUECTL_CONFIG,staging,darwin-arm64) $(MAKE) -C cmd/runectl dist-darwin-arm64

notary-credentials:
	xcrun notarytool store-credentials "$(NOTARY_PROFILE)" --team-id "YYZRWD888J"

# deps brings in git-managed prerequisites that are needed for local builds.
deps: rune-llamacpp-init ox-api-init docs-init

# rune-llamacpp-init initialises the llama.cpp git submodule that lives
# under rune/llm/llamacpp and is consumed by the rune host's local-model
# backend. Safe to run repeatedly.
#
# Guarded on `.git` so a checked-out submodule with uncommitted local
# edits is not silently reset to the superproject's pinned SHA.
rune-llamacpp-init:
	@ [ -e llm/llamacpp/llama.cpp/.git ] || git submodule update --init --recursive llm/llamacpp/llama.cpp

# ox-api-init makes sure the ox-api git submodule is checked out so the
# cmd/ox-api package compiles. Safe to run repeatedly.
#
# Guarded on `.git` so a checked-out submodule with uncommitted local
# edits is not silently reset to the superproject's pinned SHA.
ox-api-init:
	@ [ -e cmd/ox-api/.git ] || git submodule update --init --recursive cmd/ox-api

# docs-init makes sure the cmd/rune/docs git submodule is checked out so the
# //go:embed directives in cmd/rune/docs_scheme.go find the markdown sources
# that back the in-memory docs:// workspace scheme. Safe to run repeatedly.
#
# Guarded on `.git` so a checked-out submodule with uncommitted local
# edits is not silently reset to the superproject's pinned SHA.
docs-init:
	@ [ -e cmd/rune/docs/.git ] || git submodule update --init --recursive cmd/rune/docs

# rune-llamacpp-libs builds the static libraries used by rune/llm/llamacpp.
# Skipped silently when the libs are already present and fresher than the
# submodule's CMakeLists.txt — the submodule Makefile handles its own
# up-to-date checks.
$(RUNE_LLAMACPP_STAMP): rune-llamacpp-init
	@ mkdir -p $(dir $@)
	@ $(MAKE) -C llm/llamacpp libs
	@ touch $@

rune-llamacpp-libs: $(RUNE_LLAMACPP_STAMP)
