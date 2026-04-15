# syntax=docker/dockerfile:1

ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETOS
ARG TARGETARCH
ARG GIT_SSH_KEY
ARG RUNE_BUILD_TAG
ARG RUNE_BUILD_COMMIT
ARG RUNE_RELEASE_TAR

WORKDIR /src

COPY . .

ENV GOPRIVATE=github.com/unstablebuild,unstable.build/* \
	CGO_ENABLED=1

# Linux-native builder image used to cross-compile Rune for the requested
# linux/$TARGETARCH while the Go toolchain itself runs natively on the builder
# platform. The appropriate cross compiler and target-arch Linux GUI development
# packages are installed in the build stage, then cmd/rune is cross-compiled to
# $TARGETOS/$TARGETARCH in Docker.
#
# Supported targets: linux/amd64, linux/arm64
RUN set -eux; \
	if [ "$TARGETOS" != "linux" ]; then \
		echo "unsupported target OS: $TARGETOS (expected linux)"; \
		exit 1; \
	fi; \
	case "$TARGETARCH" in \
		amd64) \
			DPKG_ARCH=amd64; \
			CROSS_ESSENTIAL=crossbuild-essential-amd64; \
			;; \
		arm64) \
			DPKG_ARCH=arm64; \
			CROSS_ESSENTIAL=crossbuild-essential-arm64; \
			;; \
		*) \
			echo "unsupported target arch: $TARGETARCH (expected amd64 or arm64)"; \
			exit 1; \
			;; \
	esac; \
	dpkg --add-architecture "$DPKG_ARCH"; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		pkg-config \
		bash \
		make \
		git \
		openssh-client \
		ca-certificates \
		file \
		"$CROSS_ESSENTIAL" \
		"libasound2-dev:$DPKG_ARCH" \
		"libx11-dev:$DPKG_ARCH" \
		"libxrandr-dev:$DPKG_ARCH" \
		"libxcursor-dev:$DPKG_ARCH" \
		"libxinerama-dev:$DPKG_ARCH" \
		"libxi-dev:$DPKG_ARCH" \
		"libgl1-mesa-dev:$DPKG_ARCH" \
		"libxxf86vm-dev:$DPKG_ARCH"; \
	rm -rf /var/lib/apt/lists/*

RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/
RUN mkdir -p ~/.ssh && \
	echo "$GIT_SSH_KEY" > ~/.ssh/id_rsa && \
	chmod 600 ~/.ssh/id_rsa && \
	chmod 700 ~/.ssh
RUN ssh-keyscan -t rsa git.unstable.build >> ~/.ssh/known_hosts
RUN ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts

# Select the cross-compiler toolchain and pkg-config path based on TARGETARCH.
# Output layout mirrors Zed's convention:
#   rune.app/bin/rune
#   rune.app/lib/*.so.*
#   rune.app/share/applications/rune.desktop
#   rune.app/share/icons/hicolor/{512x512,1024x1024}/apps/rune.png
RUN --mount=type=cache,target=/root/.cache/go-build \
	set -eux; \
	: "${RUNE_BUILD_TAG:?RUNE_BUILD_TAG build arg is required}"; \
	: "${RUNE_BUILD_COMMIT:?RUNE_BUILD_COMMIT build arg is required}"; \
	case "$TARGETARCH" in \
		amd64) \
			CC=x86_64-linux-gnu-gcc; \
			CXX=x86_64-linux-gnu-g++; \
			PKG_CONFIG_PATH=/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig; \
			OBJDUMP=x86_64-linux-gnu-objdump; \
			TRIPLET=x86_64-linux-gnu; \
			;; \
		arm64) \
			CC=aarch64-linux-gnu-gcc; \
			CXX=aarch64-linux-gnu-g++; \
			PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig:/usr/share/pkgconfig; \
			OBJDUMP=aarch64-linux-gnu-objdump; \
			TRIPLET=aarch64-linux-gnu; \
			;; \
	esac; \
	export CC CXX PKG_CONFIG_PATH; \
	mkdir -p /out/rune.app/bin /out/rune.app/lib; \
	CGO_LDFLAGS='-Wl,-rpath,$ORIGIN/../lib' \
	GOOS=$TARGETOS \
	GOARCH=$TARGETARCH \
	/usr/local/go/bin/go build \
		-tags=ebitensinglethread \
		-ldflags="-X unstable.build/go-tui/debug.Tag=$RUNE_BUILD_TAG -X unstable.build/go-tui/debug.Commit=$RUNE_BUILD_COMMIT -X unstable.build/go-tui/debug.Package=rune" \
		-o /out/rune.app/bin/rune \
		./cmd/rune; \
	\
	echo "--- artifact validation ---"; \
	file /out/rune.app/bin/rune | grep 'ELF 64-bit LSB'; \
	"$OBJDUMP" -p /out/rune.app/bin/rune | awk '$1 == "NEEDED" {print $2}' > /tmp/runtime-needed.txt; \
	\
	echo "--- bundling runtime libraries into rune.app/lib/ (rpath=\$ORIGIN/../lib) ---"; \
	cp /tmp/runtime-needed.txt /tmp/lib-queue.txt; \
	while [ -s /tmp/lib-queue.txt ]; do \
		: > /tmp/lib-next.txt; \
		while IFS= read -r soname; do \
			case "$soname" in \
				libc.so*|libm.so*|libpthread.so*|libdl.so*|librt.so*|ld-linux*|linux-vdso*) continue ;; \
			esac; \
			[ -f "/out/rune.app/lib/$soname" ] && continue; \
			src=""; \
			for d in /usr/lib/$TRIPLET /lib/$TRIPLET; do \
				if [ -e "$d/$soname" ]; then src="$d/$soname"; break; fi; \
			done; \
			if [ -z "$src" ]; then \
				echo "WARN: $soname not found"; continue; \
			fi; \
			cp -L "$src" "/out/rune.app/lib/$soname"; \
			echo "  bundled: $soname"; \
			"$OBJDUMP" -p "$src" 2>/dev/null | awk '$1 == "NEEDED" {print $2}' >> /tmp/lib-next.txt; \
		done < /tmp/lib-queue.txt; \
		mv /tmp/lib-next.txt /tmp/lib-queue.txt; \
	done; \
	echo "--- bundled libraries ---"; \
	ls -la /out/rune.app/lib/ || true

# Bundle .desktop file, icons, and zdot files into the app directory.
RUN set -eux; \
	mkdir -p /out/rune.app/share/applications \
		/out/rune.app/share/icons/hicolor/512x512/apps \
		/out/rune.app/share/icons/hicolor/1024x1024/apps \
		/out/rune.app/share/zdot; \
	cp /src/deploy/rune-linux/rune.desktop /out/rune.app/share/applications/rune.desktop; \
	cp /src/extra/icon.iconset/icon_512x512.png /out/rune.app/share/icons/hicolor/512x512/apps/rune.png; \
	cp /src/extra/icon.iconset/icon_512x512@2x.png /out/rune.app/share/icons/hicolor/1024x1024/apps/rune.png; \
	cp /src/extra/osx/Rune.app/Contents/Resources/zdot/.zlogin /out/rune.app/share/zdot/; \
	cp /src/extra/osx/Rune.app/Contents/Resources/zdot/.zprofile /out/rune.app/share/zdot/; \
	cp /src/extra/osx/Rune.app/Contents/Resources/zdot/.zshenv /out/rune.app/share/zdot/; \
	cp /src/extra/osx/Rune.app/Contents/Resources/zdot/.zshrc /out/rune.app/share/zdot/

# Package the Linux bundle inside the Linux container so host-specific metadata
# such as macOS xattrs cannot be written into the release archive. Use ustar for
# broad compatibility with old and new Linux tar implementations.
RUN set -eux; \
	: "${RUNE_RELEASE_TAR:?RUNE_RELEASE_TAR build arg is required}"; \
	cd /out; \
	tar --format=ustar --no-xattrs --no-acls -czf "$RUNE_RELEASE_TAR" rune.app; \
	tar -tzf "$RUNE_RELEASE_TAR" >/dev/null

FROM scratch AS artifact
COPY --from=build /out/ /
