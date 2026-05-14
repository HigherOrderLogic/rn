ROOT_DIR ?= ../..
GO ?= go
UNAME := $(shell uname)
CODESIGN_IDENTITY ?= Developer ID Application: Unstable Build, LLC. (YYZRWD888J)
NOTARY_PROFILE ?= notary-profile

BIN := $(abspath $(ROOT_DIR)/bin)
PKG := $(abspath $(ROOT_DIR)/pkg/rune-agent)
TAR := $(abspath $(ROOT_DIR)/rune-agent.tar.gz)
NOTARIZE_ZIP := $(abspath $(ROOT_DIR)/rune-agent-notarize.zip)
DIST_SH := ./dist.sh
ROOT_DIST_SH := ./cmd/rune-agent/dist.sh

VERSION ?= $(shell git -C $(ROOT_DIR) describe --tags 2>/dev/null)
COMMIT  ?= $(shell git -C $(ROOT_DIR) rev-parse --short HEAD 2>/dev/null)
TARGET := $(abspath $(ROOT_DIR)/target)
RUNE_AGENT_LINUX_TARGET_OS ?= linux
RUNE_AGENT_LINUX_TARGET_ARCH ?= amd64
RUNE_AGENT_LINUX_OUTPUT_DIR ?= $(TARGET)/rune-agent_$(RUNE_AGENT_LINUX_TARGET_OS)_$(RUNE_AGENT_LINUX_TARGET_ARCH)
RUNE_AGENT_LINUX_RELEASE_TAR ?= rune-agent-release-$(RUNE_AGENT_LINUX_TARGET_OS)-$(RUNE_AGENT_LINUX_TARGET_ARCH)-$(VERSION).tar.gz

.PHONY: clean build pkg sign notarize dist dist-notarized \
	linux-cross-compile release-linux-amd64 release-linux-arm64 \
	dist-linux-amd64 dist-linux-arm64

build: $(BIN)/rune-agent

$(BIN)/rune-agent:
	@$(MAKE) -C $(ROOT_DIR) $(notdir $@)

clean:
	@rm -rf $(PKG) $(TAR) $(NOTARIZE_ZIP) $(BIN)/rune-agent

pkg: $(BIN)/rune-agent
	@mkdir -p $(PKG)/bin
	@cp $(BIN)/rune-agent $(PKG)/bin/
	@cp config.yaml $(PKG)/
	@cp -r skills $(PKG)/skills

ifeq ($(UNAME),Darwin)
sign: pkg
	codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(PKG)/bin/rune-agent

$(NOTARIZE_ZIP): sign
	zip $(NOTARIZE_ZIP) $(PKG)/bin/rune-agent

notarize: $(NOTARIZE_ZIP)
	xcrun notarytool submit $(NOTARIZE_ZIP) --keychain-profile "$(NOTARY_PROFILE)"

$(TAR): sign
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use rune-agent-prod-dist or rune-agent-staging-dist}
	@cd $(ROOT_DIR) && BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR $(ROOT_DIST_SH)

dist-notarized: clean notarize $(TAR)
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use rune-agent-prod-dist-notarized or rune-agent-staging-dist-notarized}
	@cd $(ROOT_DIR) && BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR $(ROOT_DIST_SH)
else
sign: pkg
	@echo "Skipping codesign (not on macOS)"

notarize: sign
	@echo "Skipping notarization (not on macOS)"

$(TAR): pkg
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use rune-agent-prod-dist or rune-agent-staging-dist}
	@cd $(ROOT_DIR) && BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR $(ROOT_DIST_SH)

dist-notarized: dist
	@echo "Skipping notarized dist (not on macOS)"
endif

# linux-cross-compile builds a stripped-down rune-agent bundle (no GUI deps)
# inside a Debian-based Docker image. The resulting tarball is dropped into
# $(RUNE_AGENT_LINUX_OUTPUT_DIR) along with the unpacked rune-agent/ tree.
linux-cross-compile:
	@rm -rf $(RUNE_AGENT_LINUX_OUTPUT_DIR)
	@mkdir -p $(RUNE_AGENT_LINUX_OUTPUT_DIR)
	@cd $(ROOT_DIR) && docker buildx build --rm \
		-f deploy/rune-agent/Dockerfile \
		--platform $(RUNE_AGENT_LINUX_TARGET_OS)/$(RUNE_AGENT_LINUX_TARGET_ARCH) \
		--build-arg GIT_SSH_KEY="$$GIT_SSH_KEY" \
		--build-arg RUNE_BUILD_TAG="$(VERSION)" \
		--build-arg RUNE_BUILD_COMMIT="$(COMMIT)" \
		--build-arg RUNE_RELEASE_TAR="$(RUNE_AGENT_LINUX_RELEASE_TAR)" \
		--output type=local,dest=$(RUNE_AGENT_LINUX_OUTPUT_DIR) \
		.

release-linux-amd64:
	@RUNE_AGENT_LINUX_TARGET_ARCH=amd64 \
		RUNE_AGENT_LINUX_RELEASE_TAR=rune-agent-release-linux-amd64-$$(git -C $(ROOT_DIR) describe --tags --dirty).tar.gz \
		$(MAKE) linux-cross-compile

release-linux-arm64:
	@RUNE_AGENT_LINUX_TARGET_ARCH=arm64 \
		RUNE_AGENT_LINUX_RELEASE_TAR=rune-agent-release-linux-arm64-$$(git -C $(ROOT_DIR) describe --tags --dirty).tar.gz \
		$(MAKE) linux-cross-compile

dist-linux-amd64: release-linux-amd64
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use rune-agent-prod-dist-linux-amd64 or rune-agent-staging-dist-linux-amd64}
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/rune-agent_linux_amd64/rune-agent-release-linux-amd64-$$(git describe --tags --dirty).tar.gz \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=amd64 \
		./cmd/rune/dist.sh

dist-linux-arm64: release-linux-arm64
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use rune-agent-prod-dist-linux-arm64 or rune-agent-staging-dist-linux-arm64}
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/rune-agent_linux_arm64/rune-agent-release-linux-arm64-$$(git describe --tags --dirty).tar.gz \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=arm64 \
		./cmd/rune/dist.sh
