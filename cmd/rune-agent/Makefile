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

.PHONY: clean build pkg sign notarize dist dist-notarized

build: $(BIN)/rune-agent

$(BIN)/rune-agent:
	@$(MAKE) -C $(ROOT_DIR) $(notdir $@)

clean:
	@rm -rf $(PKG) $(TAR) $(NOTARIZE_ZIP)

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
	@$(DIST_SH)

dist-notarized: clean notarize $(TAR)
	@$(DIST_SH)
else
sign: pkg
	@echo "Skipping codesign (not on macOS)"

notarize: sign
	@echo "Skipping notarization (not on macOS)"

$(TAR): pkg
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@$(DIST_SH)

dist-notarized: dist
	@echo "Skipping notarized dist (not on macOS)"
endif
