ROOT_DIR ?= ../..
GO ?= go
UNAME := $(shell uname)
CODESIGN_IDENTITY ?= Developer ID Application: Unstable Build, LLC. (YYZRWD888J)
NOTARY_PROFILE ?= notary-profile

BIN := $(abspath $(ROOT_DIR)/bin)
PKG := $(abspath $(ROOT_DIR)/pkg/runectl)
TAR := $(abspath $(ROOT_DIR)/runectl.tar.gz)
NOTARIZE_ZIP := $(abspath $(ROOT_DIR)/runectl-notarize.zip)
DIST_SH := ./dist.sh
ROOT_DIST_SH := ./cmd/runectl/dist.sh

.PHONY: clean build pkg sign notarize dist dist-notarized

build: $(BIN)/runectl

$(BIN)/runectl:
	@mkdir -p $(BIN)
	@GOBIN="$(BIN)" $(GO) install github.com/unstablebuild/rune-go-sdk/cmd/runectl

clean:
	@rm -rf $(PKG) $(TAR) $(NOTARIZE_ZIP) $(BIN)/runectl

pkg: $(BIN)/runectl
	@mkdir -p $(PKG)/bin
	@cp $(BIN)/runectl $(PKG)/bin/

ifeq ($(UNAME),Darwin)
sign: pkg
	codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(PKG)/bin/runectl

$(NOTARIZE_ZIP): sign
	zip $(NOTARIZE_ZIP) $(PKG)/bin/runectl

notarize: $(NOTARIZE_ZIP)
	xcrun notarytool submit $(NOTARIZE_ZIP) --keychain-profile "$(NOTARY_PROFILE)"

$(TAR): sign
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use runectl-prod-dist or runectl-staging-dist}
	@cd $(ROOT_DIR) && BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR $(ROOT_DIST_SH)

dist-notarized: clean notarize $(TAR)
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use runectl-prod-dist-notarized or runectl-staging-dist-notarized}
	@cd $(ROOT_DIR) && BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR $(ROOT_DIST_SH)
else
sign: pkg
	@echo "Skipping codesign (not on macOS)"

notarize: sign
	@echo "Skipping notarization (not on macOS)"

$(TAR): pkg
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@: $${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR not set; use runectl-prod-dist or runectl-staging-dist}
	@cd $(ROOT_DIR) && BLUECTL_CONFIG_DIR=$$BLUECTL_CONFIG_DIR $(ROOT_DIST_SH)

dist-notarized: dist
	@echo "Skipping notarized dist (not on macOS)"
endif
