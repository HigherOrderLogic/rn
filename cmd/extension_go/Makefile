ROOT_DIR ?= ../..
GO ?= go
UNAME := $(shell uname)
CODESIGN_IDENTITY ?= Developer ID Application: Unstable Build, LLC. (YYZRWD888J)
NOTARY_PROFILE ?= notary-profile

BIN := $(abspath $(ROOT_DIR)/bin)
PKG := $(abspath $(ROOT_DIR)/pkg/go)
TAR := $(abspath $(ROOT_DIR)/go.tar.gz)
NOTARIZE_ZIP := $(abspath $(ROOT_DIR)/go-notarize.zip)
DIST_SH := ./dist.sh
ROOT_DIST_SH := ./cmd/extension_go/dist.sh

.PHONY: clean build pkg sign notarize dist dist-notarized

build: $(BIN)/extension_go

$(BIN)/extension_go:
	@$(MAKE) -C $(ROOT_DIR) $(notdir $@)

clean:
	@rm -rf $(PKG) $(TAR) $(NOTARIZE_ZIP)

pkg: $(BIN)/extension_go
	@mkdir -p $(PKG)/bin
	@cp $(BIN)/extension_go $(PKG)/bin/

ifeq ($(UNAME),Darwin)
sign: pkg
	codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(PKG)/bin/extension_go

$(NOTARIZE_ZIP): sign
	zip $(NOTARIZE_ZIP) $(PKG)/bin/extension_go

notarize: $(NOTARIZE_ZIP)
	xcrun notarytool submit $(NOTARIZE_ZIP) --keychain-profile "$(NOTARY_PROFILE)"

$(TAR): sign
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@cd $(ROOT_DIR) && $(ROOT_DIST_SH)

dist-notarized: clean notarize $(TAR)
	@cd $(ROOT_DIR) && $(ROOT_DIST_SH)
else
sign: pkg
	@echo "Skipping codesign (not on macOS)"

notarize: sign
	@echo "Skipping notarization (not on macOS)"

$(TAR): pkg
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@cd $(ROOT_DIR) && $(ROOT_DIST_SH)

dist-notarized: dist
	@echo "Skipping notarized dist (not on macOS)"
endif
