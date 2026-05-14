# bluectl release configs

This directory holds the bluectl config folders used by the `rune-agent`,
`runectl`, and `fuzzy-search` release pipelines.

Layout:

- `prod/config`    — pins `auth.project-id: rune-prod`
- `staging/config` — pins `auth.project-id: unstable-build-blue-dev`

Each config sets `auth.project-id`, an empty `auth.credentials-file`,
and the `release.collection` / `release.bucket` defaults so the file
is valid. `credentials-file: ""` tells bluectl to fall back to the
developer's gcloud Application Default Credentials — credentials are
NOT committed here.

Note that `bluectl -c <dir>` replaces the config dir wholesale rather
than merging with `~/.bluectl/config`, so every field bluectl needs at
release-upload time must be present in the committed file.

## Safety property

The `*-prod-dist*` and `*-staging-dist*` make targets pass `-c
deploy/bluectl/{prod,staging}` to bluectl, so the publishing project-id
is selected by the make target rather than by whatever happens to be in
`~/.bluectl/config`. This makes it impossible for a developer's ambient
`~/.bluectl/config` to silently redirect a release upload to the wrong
environment.

If you need to add credentials, do it via gcloud
(`gcloud auth application-default login`) or the environment, never
in this directory.
