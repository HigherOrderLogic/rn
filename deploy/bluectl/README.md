# bluectl release configs

This directory holds the bluectl config folders used by the `rune-agent`,
`runectl`, and `fuzzy-search` release pipelines. Each config pins
`auth.project-id`, `release.collection`, and `release.bucket` so the
publishing destination is fully determined by the make target rather
than by `~/.bluectl/config`.

## Three keys, three roles

`bluectl release upload` reads three independent things from its config:

- `auth.project-id` — the GCP project the bluectl tool is scoped to. Sets
  the GCS / Firestore project for the run.
- `release.collection` — the Firestore collection that records the
  release manifest. Firestore collections are isolated **per project**,
  so the same collection name (`rune-release-<os>-<arch>`) is reused in
  prod and staging without collision.
- `release.bucket` — the GCS bucket the tarball is written to. GCS
  bucket names are **globally unique**, so prod and staging must use
  different names.

The earlier mix-up was treating `release.collection` as the bucket: that
caused staging uploads to land in the prod bucket because both configs
named `rune-release-darwin-arm64` and bluectl interpreted that as the
globally-unique bucket name when no `release.bucket` was set.

## Layout

```
deploy/bluectl/
  prod/
    darwin-arm64/config   bucket: rune-release-darwin-arm64
    darwin-amd64/config   bucket: rune-release-darwin-amd64
    linux-amd64/config    bucket: rune-release-linux-amd64
    linux-arm64/config    bucket: rune-release-linux-arm64
  staging/
    darwin-arm64/config   bucket: rune-dev-darwin-arm64
    darwin-amd64/config   bucket: rune-dev-darwin-amd64
    linux-amd64/config    bucket: rune-dev-linux-amd64
    linux-arm64/config    bucket: rune-dev-linux-arm64
```

Every config keeps `release.collection: rune-release-<os>-<arch>` —
Firestore namespacing by project keeps prod and staging manifests
separate.

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
