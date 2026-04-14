ROOT_DIR ?= $(abspath ../..)
GO ?= go
UNAME := $(shell uname)
VERSION=$(shell git describe --tags)
COMMIT=$(shell git rev-parse --short HEAD)
CODESIGN_IDENTITY ?= Developer ID Application: Unstable Build, LLC. (YYZRWD888J)
NOTARY_PROFILE ?= notary-profile
RUNE_LINUX_TARGET_OS ?= linux
RUNE_LINUX_TARGET_ARCH ?= amd64
RUNE_LINUX_OUTPUT_DIR ?= $(TARGET)/rune_$(RUNE_LINUX_TARGET_OS)_$(RUNE_LINUX_TARGET_ARCH)

COMMON_LDFLAGS=-X unstable.build/go-tui/debug.Tag=$$(git describe --tags) -X unstable.build/go-tui/debug.Commit=$$(git rev-parse --short HEAD)
GOFLAGS=-ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=six"
RUNE_GOFLAGS=-tags=ebitensinglethread -ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=rune"
OXAPI_GOFLAGS=-ldflags="$(COMMON_LDFLAGS) -X unstable.build/go-tui/debug.Package=rune"

TARGET := $(abspath $(ROOT_DIR)/target)
RUNE_RELEASE_DIR := $(TARGET)/rune_darwin_app
APP_NAME := Rune.app
APP_TEMPLATE := $(abspath $(ROOT_DIR)/extra/osx/$(APP_NAME))
APP_DIR := $(RUNE_RELEASE_DIR)
APP_BINARY := $(RUNE_RELEASE_DIR)/rune
APP_BINARY_DIR := $(APP_DIR)/$(APP_NAME)/Contents/MacOS
APP_EXTRAS_DIR := $(APP_DIR)/$(APP_NAME)/Contents/Resources
APP_PLIST := $(APP_DIR)/$(APP_NAME)/Contents/Info.plist
APP_ICON := $(APP_EXTRAS_DIR)/rune.icns
DMG_NAME := Rune.dmg
DMG_DIR := $(RUNE_RELEASE_DIR)
SED_INPLACE = ''

ifeq ($(OS),Darwin)
SED_INPLACE = ''
endif

.PHONY: clean make-release release release-amd64 release-arm64 app-amd64 app-arm64 dmg dmg-notarize release-all \
	docker-build docker-run docker-build-gcp docker-push-gcp linux-cross-compile \
	release-linux-amd64 release-linux-arm64 dist-linux-amd64 dist-linux-arm64 \
	dist-darwin-arm64

make-release: CGO_ENABLED=CGO_ENABLED=1
make-release:
	@mkdir -p $(TARGET)/rune_$(TARGET_OS)_$(TARGET_ARCH)
	@cd $(ROOT_DIR) && $(CGO_ENABLED) GOARCH=$(TARGET_ARCH) $(TARGET_ARCH_FLAGS) GOOS=$(TARGET_OS) $(GO) build $(RUNE_GOFLAGS) -o $(TARGET)/rune_$(TARGET_OS)_$(TARGET_ARCH)/rune ./cmd/rune
	@cd $(ROOT_DIR) && CGO_ENABLED=0 GOARCH=$(TARGET_ARCH) $(TARGET_ARCH_FLAGS) GOOS=$(TARGET_OS) $(GO) build $(OXAPI_GOFLAGS) -o $(TARGET)/rune_$(TARGET_OS)_$(TARGET_ARCH)/ox-api ./cmd/ox-api
	@cd $(ROOT_DIR) && CGO_ENABLED=0 GOARCH=$(TARGET_ARCH) $(TARGET_ARCH_FLAGS) GOOS=$(TARGET_OS) $(GO) build $(GOFLAGS) -o $(TARGET)/rune_$(TARGET_OS)_$(TARGET_ARCH)/claudeimport ./cmd/claudeimport
ifeq ($(UNAME),Linux)
release:
	@rm -rf $(TARGET)/rune_linux_amd64
	@TARGET_OS=linux TARGET_ARCH=amd64 $(MAKE) make-release
	@cd $(TARGET) && tar -czvf rune-release-`git describe --tags --dirty`.tar.gz rune_linux_amd64

app-amd64:
	@echo "Skipping app packaging (not macOS)"

app-arm64:
	@echo "Skipping app packaging (not macOS)"

dmg:
	@echo "Skipping DMG packaging (not macOS)"

dmg-notarize: dmg
	@echo "Skipping DMG notarization (not macOS)"

release-all:
	@echo "Skipping release-all (not macOS)"
endif

ifeq ($(UNAME),Darwin)
release-amd64:
	@rm -rf $(TARGET)/rune_darwin_amd64
	@TARGET_OS=darwin TARGET_ARCH=amd64 $(MAKE) make-release
	@cd $(TARGET) && tar -czvf rune-release-`git describe --tags --dirty`.tar.gz rune_darwin_amd64

release-arm64:
	@rm -rf $(TARGET)/rune_darwin_arm64
	@TARGET_OS=darwin TARGET_ARCH=arm64 $(MAKE) make-release
	@cd $(TARGET) && tar -czvf rune-release-`git describe --tags --dirty`.tar.gz rune_darwin_arm64

release:
	@rm -rf $(TARGET)/rune_darwin_arm64 $(TARGET)/rune_darwin_amd64
	@TARGET_OS=darwin TARGET_ARCH=arm64 $(MAKE) make-release
	@TARGET_OS=darwin TARGET_ARCH=amd64 $(MAKE) make-release
	@cd $(TARGET) && tar -czvf rune-release-`git describe --tags --dirty`.tar.gz rune_darwin_arm64 rune_darwin_amd64

app-amd64:
	@rm -rf $(RUNE_RELEASE_DIR)
	@mkdir -p $(RUNE_RELEASE_DIR)
	@cd $(ROOT_DIR) && CGO_ENABLED=1 GOARCH=amd64 GOOS=darwin $(GO) build $(RUNE_GOFLAGS) -o $(APP_BINARY) ./cmd/rune
	@mkdir -p $(APP_BINARY_DIR) $(APP_EXTRAS_DIR)
	@cp -fRp $(APP_TEMPLATE) $(APP_DIR)
	@cp -fp $(APP_BINARY) $(APP_BINARY_DIR)
	@ibtool --compile $(APP_EXTRAS_DIR)/MainMenu.nib $(APP_TEMPLATE)/Contents/Resources/MainMenu.xib
	@echo "injecting version $(VERSION) ($(COMMIT))"
	@sed -i $(SED_INPLACE) -e 's/{{VERSION}}/$(VERSION)/g' -e 's/{{COMMIT}}/$(COMMIT)/g' $(APP_PLIST)
	@cp -fp $(APP_BINARY) $(APP_EXTRAS_DIR)/rune-extension
	@cd $(ROOT_DIR)/extra && iconutil -c icns icon.iconset
	@mv $(ROOT_DIR)/extra/icon.icns $(APP_ICON)
	@touch -r "$(APP_BINARY)" "$(APP_DIR)/$(APP_NAME)"
	@rm -f $(APP_BINARY)
	@codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" "$(APP_EXTRAS_DIR)/rune-extension"
	@codesign --force --deep --options runtime --sign "$(CODESIGN_IDENTITY)" "$(APP_DIR)/$(APP_NAME)"
	@echo "Created '$(APP_NAME)' in '$(APP_DIR)'"

app-arm64:
	@rm -rf $(RUNE_RELEASE_DIR)
	@mkdir -p $(RUNE_RELEASE_DIR)
	@cd $(ROOT_DIR) && CGO_ENABLED=1 GOARCH=arm64 GOOS=darwin $(GO) build $(RUNE_GOFLAGS) -o $(APP_BINARY) ./cmd/rune
	@mkdir -p $(APP_BINARY_DIR) $(APP_EXTRAS_DIR)
	@cp -fRp $(APP_TEMPLATE) $(APP_DIR)
	@cp -fp $(APP_BINARY) $(APP_BINARY_DIR)
	@ibtool --compile $(APP_EXTRAS_DIR)/MainMenu.nib $(APP_TEMPLATE)/Contents/Resources/MainMenu.xib
	@echo "injecting version $(VERSION) ($(COMMIT))"
	@sed -i $(SED_INPLACE) -e 's/{{VERSION}}/$(VERSION)/g' -e 's/{{COMMIT}}/$(COMMIT)/g' $(APP_PLIST)
	@cp -fp $(APP_BINARY) $(APP_EXTRAS_DIR)/rune-extension
	@cd $(ROOT_DIR)/extra && iconutil -c icns icon.iconset
	@mv $(ROOT_DIR)/extra/icon.icns $(APP_ICON)
	@touch -r "$(APP_BINARY)" "$(APP_DIR)/$(APP_NAME)"
	@rm -f $(APP_BINARY)
	@codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" "$(APP_EXTRAS_DIR)/rune-extension"
	@codesign --force --deep --options runtime --sign "$(CODESIGN_IDENTITY)" "$(APP_DIR)/$(APP_NAME)"
	@echo "Created '$(APP_NAME)' in '$(APP_DIR)'"

dmg: app-arm64
	@echo "Packing disk image..."
	@mkdir -p $(DMG_DIR)
	@ln -sf /Applications $(DMG_DIR)/Applications
	@hdiutil create $(DMG_DIR)/$(DMG_NAME) \
		-volname "Rune" \
		-fs HFS+ \
		-srcfolder $(APP_DIR) \
		-ov -format UDZO
	@echo "Signing disk image..."
	@codesign --force --options runtime --sign "$(CODESIGN_IDENTITY)" $(DMG_DIR)/$(DMG_NAME)
	@echo "Packed $(DMG_DIR)/$(DMG_NAME)"

dmg-notarize: dmg
	@echo "Notarizing disk image..."
	@set -e; \
	RESULT=$$(xcrun notarytool submit $(DMG_DIR)/$(DMG_NAME) --keychain-profile "$(NOTARY_PROFILE)" --wait --output-format json); \
	echo "$$RESULT"; \
	ID=$$(printf '%s\n' "$$RESULT" | plutil -extract id raw -o - -); \
	STATUS=$$(printf '%s\n' "$$RESULT" | plutil -extract status raw -o - -); \
	echo "Submission ID: $$ID"; \
	echo "Status: $$STATUS"; \
	if [ "$$STATUS" != "Accepted" ]; then \
		echo "Fetching notarization log..."; \
		xcrun notarytool log "$$ID" --keychain-profile "$(NOTARY_PROFILE)"; \
		exit 1; \
	fi
	@echo "Stapling ticket..."
	@xcrun stapler staple $(DMG_DIR)/$(DMG_NAME)
	@echo "Validating staple..."
	@xcrun stapler validate $(DMG_DIR)/$(DMG_NAME)
	@echo "Notarization completed: $(DMG_DIR)/$(DMG_NAME)"

release-all: dmg-notarize
endif

docker-build:
	@cd $(ROOT_DIR) && docker build -f deploy/ox-api/Dockerfile -t unstable.build/ox-api:$(VERSION) --build-arg GIT_SSH_KEY="$$GIT_SSH_KEY" .

docker-run:
	@docker run -ti --rm -e PORT=80 -p 8080:80 unstable.build/ox-api:$(VERSION)

docker-build-gcp:
	@cd $(ROOT_DIR) && docker buildx build -f deploy/ox-api/Dockerfile --platform linux/amd64 -t us-central1-docker.pkg.dev/unstable-build-blue-dev/docker/ox-api:$(VERSION) --build-arg GIT_SSH_KEY="$$GIT_SSH_KEY" .

docker-push-gcp:
	@docker push us-central1-docker.pkg.dev/unstable-build-blue-dev/docker/ox-api:$(VERSION)

linux-cross-compile:
	@rm -rf $(RUNE_LINUX_OUTPUT_DIR)
	@mkdir -p $(RUNE_LINUX_OUTPUT_DIR)
	@cd $(ROOT_DIR) && docker buildx build --rm \
		-f deploy/rune-linux/Dockerfile \
		--platform $(RUNE_LINUX_TARGET_OS)/$(RUNE_LINUX_TARGET_ARCH) \
		--build-arg GIT_SSH_KEY="$$GIT_SSH_KEY" \
		--build-arg RUNE_BUILD_TAG="$(VERSION)" \
		--build-arg RUNE_BUILD_COMMIT="$(COMMIT)" \
		--output type=local,dest=$(RUNE_LINUX_OUTPUT_DIR) \
		.

release-linux-amd64:
	@RUNE_LINUX_TARGET_ARCH=amd64 $(MAKE) linux-cross-compile
	@cd $(TARGET)/rune_linux_amd64 && tar -czvf $(TARGET)/rune-release-linux-amd64-$$(git describe --tags --dirty).tar.gz rune.app

release-linux-arm64:
	@RUNE_LINUX_TARGET_ARCH=arm64 $(MAKE) linux-cross-compile
	@cd $(TARGET)/rune_linux_arm64 && tar -czvf $(TARGET)/rune-release-linux-arm64-$$(git describe --tags --dirty).tar.gz rune.app

dist-linux-amd64: release-linux-amd64
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/rune-release-linux-amd64-$$(git describe --tags --dirty).tar.gz \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=amd64 \
		./cmd/rune/dist.sh

dist-linux-arm64: release-linux-arm64
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(TARGET)/rune-release-linux-arm64-$$(git describe --tags --dirty).tar.gz \
		BLUE_TARGET_OS=linux \
		BLUE_TARGET_ARCH=arm64 \
		./cmd/rune/dist.sh

dist-darwin-arm64: dmg-notarize
	@cd $(ROOT_DIR) && \
		BLUE_RELEASE_TAR=$(DMG_DIR)/$(DMG_NAME) \
		BLUE_TARGET_OS=darwin \
		BLUE_TARGET_ARCH=arm64 \
		./cmd/rune/dist.sh

clean:
	@rm -rf $(RUNE_RELEASE_DIR)
