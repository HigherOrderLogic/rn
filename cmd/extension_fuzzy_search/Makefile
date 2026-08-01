ROOT_DIR ?= ../..
GO ?= go
UNAME := $(shell uname)
CODESIGN_IDENTITY ?= Developer ID Application: Unstable Build, LLC. (YYZRWD888J)
NOTARY_PROFILE ?= notary-profile

BIN := $(abspath $(ROOT_DIR)/bin)
PKG := $(abspath $(ROOT_DIR)/pkg/fuzzy-search)
TAR := $(abspath $(ROOT_DIR)/fuzzy-search.tar.gz)
ROOT_DIST_SH := ./cmd/extension_fuzzy_search/dist.sh

VERSION ?= $(shell git -C $(ROOT_DIR) describe --tags 2>/dev/null)
COMMIT  ?= $(shell git -C $(ROOT_DIR) rev-parse --short HEAD 2>/dev/null)
TARGET := $(abspath $(ROOT_DIR)/target)

# HOST_ARCH is the host's Go arch. On Linux a dist target whose arch matches
# HOST_ARCH builds with the host toolchain (no Docker); other combinations
# fall back to the Docker cross-compile path.
HOST_ARCH := $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')

FUZZY_LINUX_TARGET_OS ?= linux
FUZZY_LINUX_TARGET_ARCH ?= amd64
FUZZY_LINUX_OUTPUT_DIR ?= $(TARGET)/fuzzy-search_$(FUZZY_LINUX_TARGET_OS)_$(FUZZY_LINUX_TARGET_ARCH)
FUZZY_LINUX_RELEASE_TAR ?= fuzzy-search-release-$(FUZZY_LINUX_TARGET_OS)-$(FUZZY_LINUX_TARGET_ARCH)-$(VERSION).tar.gz

# Build flags mirror the root Makefile's six package stamp.
COMMON_LDFLAGS := -X unstable.build/go-tui/debug.Tag=$(VERSION) -X unstable.build/go-tui/debug.Commit=$(COMMIT) -X unstable.build/go-tui/debug.Package=six

# darwin amd64 cross-builds on an Apple Silicon host need an explicit
# -arch x86_64 cgo toolchain; arm64 is native and needs no extra flags.
DARWIN_AMD64_TARGET_ARCH_FLAGS ?= CC="clang -arch x86_64" CGO_CFLAGS="-arch x86_64" CGO_CXXFLAGS="-arch x86_64" CGO_LDFLAGS="-arch x86_64"

.PHONY: clean build pkg dist \
	linux-cross-compile make-release-linux \
	release-linux-amd64 release-linux-arm64 \
	release-linux-amd64-cross release-linux-arm64-cross \
	dist-linux-amd64 dist-linux-arm64 \
	dist-linux-amd64-cross dist-linux-arm64-cross \
	pkg-darwin-amd64 pkg-darwin-arm64 \
	sign-darwin-amd64 sign-darwin-arm64 \
	notarize-darwin-amd64 notarize-darwin-arm64 \
	dist-darwin-amd64 dist-darwin-arm64

build: $(BIN)/extension_fuzzy_search

$(BIN)/extension_fuzzy_search:
	@$(MAKE) -C $(ROOT_DIR) bin/$(notdir $@)

clean:
	@rm -rf $(PKG) $(TAR) $(BIN)/extension_fuzzy_search $(TARGET)/fuzzy-search_*

pkg: $(BIN)/extension_fuzzy_search
	@mkdir -p $(PKG)/bin
	@cp $(BIN)/extension_fuzzy_search $(PKG)/bin/
	@cp config.star $(PKG)/
	@cp fuzzy_search.star $(PKG)/
	@go_files=$$(find $(PKG) -name '*.go'); \
		if [ -n "$$go_files" ]; then \
			echo "error: .go files found in package:" >&2; \
			echo "$$go_files" >&2; \
			exit 1; \
		fi
$(TAR): pkg
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use fuzzy-search-prod-dist or fuzzy-search-staging-dist}
	@cd $(ROOT_DIR) && BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR $(ROOT_DIST_SH)

# make-release-linux builds the fuzzy-search bundle with the host's own
# toolchain (no Docker/QEMU), mirroring the layout produced by
# deploy/fuzzy-search/Dockerfile:
#   fuzzy-search/bin/extension_fuzzy_search
#   fuzzy-search/lib/*.so.*     (transitive NEEDED libs, rpath=$ORIGIN/../lib)
#   fuzzy-search/config.star
# The tarball is rooted at the bundle contents (bin/, lib/, config.star), not
# the fuzzy-search/ wrapper, so config.star sits at the archive root where the
# installer looks for it.
# CGO is required (tree-sitter), so the binary links the host's shared libs.
# Native builds only support host-arch == target-arch; cross-arch must use the
# *-cross (Docker) targets.
make-release-linux:
	@if [ "$(UNAME)" != "Linux" ] || [ "$(HOST_ARCH)" != "$(FUZZY_LINUX_TARGET_ARCH)" ]; then \
		echo "make-release-linux requires a Linux host whose arch matches FUZZY_LINUX_TARGET_ARCH=$(FUZZY_LINUX_TARGET_ARCH) (host: $(UNAME)/$(HOST_ARCH))."; \
		echo "Use the *-cross targets (e.g. release-linux-$(FUZZY_LINUX_TARGET_ARCH)-cross) for cross-arch builds via Docker."; \
		exit 1; \
	fi
	@rm -rf $(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search
	@mkdir -p $(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search/bin $(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search/lib
	@cd $(ROOT_DIR) && CGO_ENABLED=1 CGO_LDFLAGS='-Wl,-rpath,$$ORIGIN/../lib' \
		GOOS=linux GOARCH=$(FUZZY_LINUX_TARGET_ARCH) \
		$(GO) build -ldflags="$(COMMON_LDFLAGS)" \
		-o $(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search/bin/extension_fuzzy_search ./cmd/extension_fuzzy_search
	@set -eu; \
	BIN=$(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search/bin/extension_fuzzy_search; \
	LIB=$(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search/lib; \
	TRIPLET=$$(gcc -dumpmachine); \
	objdump -p "$$BIN" | awk '$$1 == "NEEDED" {print $$2}' > "$$LIB/.queue"; \
	while [ -s "$$LIB/.queue" ]; do \
		: > "$$LIB/.next"; \
		while IFS= read -r soname; do \
			case "$$soname" in \
				libc.so*|libm.so*|libpthread.so*|libdl.so*|librt.so*|libresolv.so*|ld-linux*|linux-vdso*) continue ;; \
			esac; \
			[ -f "$$LIB/$$soname" ] && continue; \
			src=""; \
			for d in /usr/lib/$$TRIPLET /lib/$$TRIPLET /usr/lib /lib; do \
				if [ -e "$$d/$$soname" ]; then src="$$d/$$soname"; break; fi; \
			done; \
			if [ -z "$$src" ]; then echo "WARN: $$soname not found"; continue; fi; \
			cp -L "$$src" "$$LIB/$$soname"; \
			echo "  bundled: $$soname"; \
			objdump -p "$$src" 2>/dev/null | awk '$$1 == "NEEDED" {print $$2}' >> "$$LIB/.next"; \
		done < "$$LIB/.queue"; \
		mv "$$LIB/.next" "$$LIB/.queue"; \
	done; \
	rm -f "$$LIB/.queue" "$$LIB/.next"
	@cp config.star $(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search/config.star
	@cp fuzzy_search.star $(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search/fuzzy_search.star
	@cd $(FUZZY_LINUX_OUTPUT_DIR)/fuzzy-search && \
		tar --format=ustar -czf $(FUZZY_LINUX_OUTPUT_DIR)/$(FUZZY_LINUX_RELEASE_TAR) .

# linux-cross-compile builds the fuzzy-search bundle inside a Debian-based
# Docker image (used when the host cannot natively target the requested arch).
# The tarball is dropped into $(FUZZY_LINUX_OUTPUT_DIR) along with the
# unpacked fuzzy-search/ tree.
linux-cross-compile:
	@rm -rf $(FUZZY_LINUX_OUTPUT_DIR)
	@mkdir -p $(FUZZY_LINUX_OUTPUT_DIR)
	@cd $(ROOT_DIR) && docker buildx build --rm \
		-f deploy/fuzzy-search/Dockerfile \
		--platform $(FUZZY_LINUX_TARGET_OS)/$(FUZZY_LINUX_TARGET_ARCH) \
		--build-arg GIT_SSH_KEY="$$GIT_SSH_KEY" \
		--build-arg RUNE_BUILD_TAG="$(VERSION)" \
		--build-arg RUNE_BUILD_COMMIT="$(COMMIT)" \
		--build-arg RUNE_RELEASE_TAR="$(FUZZY_LINUX_RELEASE_TAR)" \
		--output type=local,dest=$(FUZZY_LINUX_OUTPUT_DIR) \
		.

# release-linux-{amd64,arm64} build natively on a matching Linux host. Use the
# *-cross variants for cross-arch builds via Docker.
release-linux-amd64:
	@FUZZY_LINUX_TARGET_ARCH=amd64 \
		FUZZY_LINUX_RELEASE_TAR=fuzzy-search-release-linux-amd64-$$(git -C $(ROOT_DIR) describe --tags --dirty).tar.gz \
		$(MAKE) make-release-linux

release-linux-arm64:
	@FUZZY_LINUX_TARGET_ARCH=arm64 \
		FUZZY_LINUX_RELEASE_TAR=fuzzy-search-release-linux-arm64-$$(git -C $(ROOT_DIR) describe --tags --dirty).tar.gz \
		$(MAKE) make-release-linux

release-linux-amd64-cross:
	@FUZZY_LINUX_TARGET_ARCH=amd64 \
		FUZZY_LINUX_RELEASE_TAR=fuzzy-search-release-linux-amd64-$$(git -C $(ROOT_DIR) describe --tags --dirty).tar.gz \
		$(MAKE) linux-cross-compile

release-linux-arm64-cross:
	@FUZZY_LINUX_TARGET_ARCH=arm64 \
		FUZZY_LINUX_RELEASE_TAR=fuzzy-search-release-linux-arm64-$$(git -C $(ROOT_DIR) describe --tags --dirty).tar.gz \
		$(MAKE) linux-cross-compile

dist-linux-amd64: release-linux-amd64
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use fuzzy-search-prod-dist-linux-amd64 or fuzzy-search-staging-dist-linux-amd64}
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/fuzzy-search_linux_amd64/fuzzy-search-release-linux-amd64-$$(git describe --tags --dirty).tar.gz \
		BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=amd64 \
		$(ROOT_DIST_SH)

dist-linux-arm64: release-linux-arm64
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use fuzzy-search-prod-dist-linux-arm64 or fuzzy-search-staging-dist-linux-arm64}
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/fuzzy-search_linux_arm64/fuzzy-search-release-linux-arm64-$$(git describe --tags --dirty).tar.gz \
		BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=arm64 \
		$(ROOT_DIST_SH)

# dist-linux-*-cross mirror dist-linux-* but build the tarball through the
# Docker cross-compile path (release-linux-*-cross). The tarball path is
# identical.
dist-linux-amd64-cross: release-linux-amd64-cross
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use fuzzy-search-prod-dist-linux-amd64-cross or fuzzy-search-staging-dist-linux-amd64-cross}
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/fuzzy-search_linux_amd64/fuzzy-search-release-linux-amd64-$$(git describe --tags --dirty).tar.gz \
		BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=amd64 \
		$(ROOT_DIST_SH)

dist-linux-arm64-cross: release-linux-arm64-cross
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use fuzzy-search-prod-dist-linux-arm64-cross or fuzzy-search-staging-dist-linux-arm64-cross}
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/fuzzy-search_linux_arm64/fuzzy-search-release-linux-arm64-$$(git describe --tags --dirty).tar.gz \
		BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=arm64 \
		$(ROOT_DIST_SH)

# pkg-darwin-{amd64,arm64} cross-build the extension binary for a specific
# macOS arch and stage config.star next to it. darwin amd64 cross-compiles
# natively on Apple Silicon via the -arch x86_64 cgo toolchain.
define fuzzy_pkg_darwin
	@rm -rf $(TARGET)/fuzzy-search_darwin_$(1)
	@mkdir -p $(TARGET)/fuzzy-search_darwin_$(1)/bin
	@cd $(ROOT_DIR) && CGO_ENABLED=1 GOOS=darwin GOARCH=$(1) $(2) \
		$(GO) build -ldflags="$(COMMON_LDFLAGS)" \
		-o $(TARGET)/fuzzy-search_darwin_$(1)/bin/extension_fuzzy_search ./cmd/extension_fuzzy_search
	@cp config.star $(TARGET)/fuzzy-search_darwin_$(1)/
	@cp fuzzy_search.star $(TARGET)/fuzzy-search_darwin_$(1)/
	@go_files=$$(find $(TARGET)/fuzzy-search_darwin_$(1) -name '*.go'); \
		if [ -n "$$go_files" ]; then \
			echo "error: .go files found in package:" >&2; \
			echo "$$go_files" >&2; \
			exit 1; \
		fi
endef

pkg-darwin-amd64:
	$(call fuzzy_pkg_darwin,amd64,$(DARWIN_AMD64_TARGET_ARCH_FLAGS))

pkg-darwin-arm64:
	$(call fuzzy_pkg_darwin,arm64,)

ifeq ($(UNAME),Darwin)
sign-darwin-amd64: pkg-darwin-amd64
	codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(TARGET)/fuzzy-search_darwin_amd64/bin/extension_fuzzy_search

sign-darwin-arm64: pkg-darwin-arm64
	codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(TARGET)/fuzzy-search_darwin_arm64/bin/extension_fuzzy_search

notarize-darwin-amd64: sign-darwin-amd64
	zip $(TARGET)/fuzzy-search-notarize-darwin-amd64.zip $(TARGET)/fuzzy-search_darwin_amd64/bin/extension_fuzzy_search
	xcrun notarytool submit $(TARGET)/fuzzy-search-notarize-darwin-amd64.zip --keychain-profile "$(NOTARY_PROFILE)"

notarize-darwin-arm64: sign-darwin-arm64
	zip $(TARGET)/fuzzy-search-notarize-darwin-arm64.zip $(TARGET)/fuzzy-search_darwin_arm64/bin/extension_fuzzy_search
	xcrun notarytool submit $(TARGET)/fuzzy-search-notarize-darwin-arm64.zip --keychain-profile "$(NOTARY_PROFILE)"
else
sign-darwin-amd64: pkg-darwin-amd64
	@echo "Skipping codesign (not on macOS)"

sign-darwin-arm64: pkg-darwin-arm64
	@echo "Skipping codesign (not on macOS)"

notarize-darwin-amd64: sign-darwin-amd64
	@echo "Skipping notarization (not on macOS)"

notarize-darwin-arm64: sign-darwin-arm64
	@echo "Skipping notarization (not on macOS)"
endif

dist-darwin-amd64: sign-darwin-amd64
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use fuzzy-search-prod-dist-darwin-amd64 or fuzzy-search-staging-dist-darwin-amd64}
	@cd $(TARGET)/fuzzy-search_darwin_amd64 && tar -czvf $(TARGET)/fuzzy-search-darwin-amd64.tar.gz .
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/fuzzy-search-darwin-amd64.tar.gz \
		BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR \
		BLUE_TARGET_OS=darwin \
		BLUE_TARGET_ARCH=amd64 \
		$(ROOT_DIST_SH)

dist-darwin-arm64: sign-darwin-arm64
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use fuzzy-search-prod-dist-darwin-arm64 or fuzzy-search-staging-dist-darwin-arm64}
	@cd $(TARGET)/fuzzy-search_darwin_arm64 && tar -czvf $(TARGET)/fuzzy-search-darwin-arm64.tar.gz .
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/fuzzy-search-darwin-arm64.tar.gz \
		BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR \
		BLUE_TARGET_OS=darwin \
		BLUE_TARGET_ARCH=arm64 \
		$(ROOT_DIST_SH)
