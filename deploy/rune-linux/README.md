# Rune Linux cross-compilation

`deploy/rune-linux/Dockerfile` cross-compiles the Rune GUI binary for
`linux/amd64` or `linux/arm64` inside Docker without forcing the Go
toolchain itself to run under target-arch emulation.

## Supported targets

| Target | Cross compiler | pkg-config path |
|---|---|---|
| `linux/amd64` | `x86_64-linux-gnu-gcc` | `/usr/lib/x86_64-linux-gnu/pkgconfig` |
| `linux/arm64` | `aarch64-linux-gnu-gcc` | `/usr/lib/aarch64-linux-gnu/pkgconfig` |

## How it works

The `make rune-linux-cross-compile` rule works by:

1. building `deploy/rune-linux/Dockerfile` with `docker buildx`
2. running the Go toolchain on `$BUILDPLATFORM`
3. passing `GIT_SSH_KEY` with the same build-arg SSH setup used by
   `deploy/build/Dockerfile`
4. installing the target-arch Linux cross compiler and development headers
5. cross-compiling `./cmd/rune` to `linux/$TARGETARCH` with
   `rpath=$ORIGIN/../lib` so the binary finds its bundled libraries
6. copying each NEEDED shared library (and their transitive deps) into
   `rune.app/lib/`, following symlinks so every file is a real ELF object
7. packaging `rune.app/` into a `ustar` `.tar.gz` inside the Linux container
   so host-specific metadata such as macOS xattrs cannot enter the archive
8. exporting both the `rune.app/` directory and `.tar.gz` from the final
   scratch stage

## Build commands

Build for `linux/amd64` (default):

```bash
make rune-linux-cross-compile
# or explicitly:
make rune-linux-cross-compile RUNE_LINUX_TARGET_ARCH=amd64
```

Build for `linux/arm64`:

```bash
make rune-linux-cross-compile RUNE_LINUX_TARGET_ARCH=arm64
```

## Release tarballs

To build and package a release tarball:

```bash
make rune-release-linux-amd64   # -> target/rune_linux_amd64/rune-release-linux-amd64-<tag>.tar.gz
make rune-release-linux-arm64   # -> target/rune_linux_arm64/rune-release-linux-arm64-<tag>.tar.gz
```

To build, package, and upload via `bluectl release upload`:

```bash
make rune-dist-linux-amd64
make rune-dist-linux-arm64
```

## Release layout

The tarball and cross-compile output follow the same `.app` directory
convention used by Zed:

```
rune.app/
  bin/
    rune            # the main binary (rpath = $ORIGIN/../lib)
  lib/
    libX11.so.6     # bundled shared libraries
    ...
  share/
    applications/
      rune.desktop  # freedesktop .desktop entry
    icons/
      hicolor/
        512x512/apps/rune.png
        1024x1024/apps/rune.png
    zdot/
      .zlogin         # zsh dot files for integrated terminal
      .zprofile
      .zshenv
      .zshrc
```

## System library requirements

The binary bundles its direct NEEDED shared libraries and their
transitive dependencies in `rune.app/lib/`. glibc core libraries
(`libc`, `libm`, `libpthread`, `libdl`, `librt`, `ld-linux`) are
**not** bundled and must be provided by the host system.

Any reasonably recent Linux distribution includes these. Headless
servers without X11/OpenGL libraries may not work for Rune's GUI.
