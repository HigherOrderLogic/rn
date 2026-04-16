ROOT_DIR ?= ../..
GO ?= go

BIN := $(abspath $(ROOT_DIR)/bin)
PKG := $(abspath $(ROOT_DIR)/pkg/extension_fuzzy_search)
TAR := $(abspath $(ROOT_DIR)/extension_fuzzy_search.tar.gz)
ROOT_DIST_SH := ./cmd/extension_fuzzy_search/dist.sh

.PHONY: clean build pkg dist

build: $(BIN)/extension_fuzzy_search

$(BIN)/extension_fuzzy_search:
	@$(MAKE) -C $(ROOT_DIR) bin/$(notdir $@)

clean:
	@rm -rf $(PKG) $(TAR) $(BIN)/extension_fuzzy_search

pkg: $(BIN)/extension_fuzzy_search
	@mkdir -p $(PKG)/bin
	@cp $(BIN)/extension_fuzzy_search $(PKG)/bin/
	@cp config.yaml $(PKG)/
$(TAR): pkg
	cd $(PKG) && tar -czvf $(TAR) .

dist: clean $(TAR)
	@cd $(ROOT_DIR) && $(ROOT_DIST_SH)
