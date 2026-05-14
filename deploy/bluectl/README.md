# bluectl release configs

This directory holds the bluectl config folders used by the `rune-agent`,
`runectl`, and `fuzzy-search` release pipelines. Each config pins both
`auth.project-id` and `release.collection` so the publishing destination
is fully determined by the make target rather than by `~/.bluectl/config`.

## Why the bucket matters

`bluectl release upload` writes the artifact to the GCS bucket whose name
matches `release.collection` (or `release.bucket` if set). Bucket names
are globally unique on GCS — `rune-release-darwin-arm64` is a prod-owned
bucket, regardless of which project's credentials are presenting the
request. **Pinning `auth.project-id` alone is not enough**: if
`release.collection` resolves to a prod-owned bucket, the bytes land in
prod.

These configs pin BOTH:

- `auth.project-id` — which project the release manifest is recorded
  against (`rune-prod` vs `unstable-build-blue-dev`).
- `release.collection` — which GCS bucket the tarball is uploaded to.

## Layout

```
deploy/bluectl/
  prod/
    darwin-arm64/config   collection: rune-release-darwin-arm64
    darwin-amd64/config   collection: rune-release-darwin-amd64
    linux-amd64/config    collection: rune-release-linux-amd64
    linux-arm64/config    collection: rune-release-linux-arm64
  staging/
    darwin-arm64/config   collection: rune-dev-darwin-arm64
    darwin-amd64/config   collection: rune-dev-darwin-amd64
    linux-amd64/config    collection: rune-dev-linux-amd64
    linux-arm64/config    collection: rune-dev-linux-arm64
```

`credentials-file: ""` tells bluectl to fall back to gcloud Application
Default Credentials. No secrets are committed here.

## Wiring

The make targets resolve a leaf config dir from `BLUECTL_ENV`,
`BLUECTL_TARGET_OS`, and `BLUECTL_TARGET_ARCH` and pass it to
`bluectl -c <dir>`:

```
rune-agent-prod-dist            -> deploy/bluectl/prod/<host-os>-<host-arch>
rune-agent-staging-dist         -> deploy/bluectl/staging/<host-os>-<host-arch>
rune-agent-prod-dist-linux-amd64 -> deploy/bluectl/prod/linux-amd64
rune-agent-staging-dist-linux-arm64 -> deploy/bluectl/staging/linux-arm64
...etc.
```

`bluectl -c <dir>` swaps the config wholesale rather than merging with
`~/.bluectl/config`, so every field needed at upload time must be
present in the committed file.

## Adding credentials

Run `gcloud auth application-default login` against the right account
(or use a service-account env var). Do not put credentials in this
directory.
