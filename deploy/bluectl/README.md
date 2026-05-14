# bluectl release configs

This directory holds the bluectl config folders used by the `rune-agent`,
`runectl`, and `fuzzy-search` release pipelines.

Layout:

- `prod/config`    — pins `auth.project-id: rune-prod`
- `staging/config` — pins `auth.project-id: unstable-build-blue-dev`

Each config sets only `auth.project-id` (plus harmless `release.collection`
/ `release.bucket` defaults so the file is valid). Credentials are NOT
committed here: bluectl falls back to the developer's gcloud
Application Default Credentials.

## Safety property

The `*-prod-dist*` and `*-staging-dist*` make targets pass `-c
deploy/bluectl/{prod,staging}` to bluectl, so the publishing project-id
is selected by the make target rather than by whatever happens to be in
`~/.bluectl/config`. This makes it impossible for a developer's ambient
`~/.bluectl/config` to silently redirect a release upload to the wrong
environment.

If you need to add credentials or other auth fields, do it via your
personal `~/.bluectl/config` or the environment — never in this
directory.
