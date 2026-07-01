#!/usr/bin/env bash
#
# Build a styled Rune DMG using the vendored create-dmg, with a HiDPI
# background, a branded volume icon, and a fixed drag-to-Applications
# layout. Any failure is fatal (non-zero exit): a release must never
# silently ship an unstyled DMG. Callers that intentionally want a plain
# image use the Makefile's DMG_STYLED=0 path, which bypasses this script.
#
# All inputs are explicit arguments (no hidden globals):
#
#   build-dmg.sh <app_dir> <output_dmg> <volname>
#
#   app_dir     directory whose contents are copied into the volume; must
#               contain exactly one *.app at its root.
#   output_dmg  path of the .dmg to (re)create.
#   volname     Finder volume name (window title + sidebar label).
#
# The script locates its assets relative to its own directory, so it does
# not depend on the caller's working directory.

set -euo pipefail

if [[ $# -ne 3 ]]; then
	echo >&2 "usage: $(basename "$0") <app_dir> <output_dmg> <volname>"
	exit 2
fi

APP_DIR="$1"
OUTPUT_DMG="$2"
VOLNAME="$3"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CREATE_DMG="$SCRIPT_DIR/create-dmg/create-dmg"
BACKGROUND_1X="$SCRIPT_DIR/dmg-background.png"
BACKGROUND_2X="$SCRIPT_DIR/dmg-background@2x.png"
ICONSET_DIR="$(cd "$SCRIPT_DIR/.." && pwd)/icon.iconset"

# Window / icon layout. Kept in sync with the background art dimensions:
# the @1x background is WINDOW_W x WINDOW_H, the @2x is exactly double.
WINDOW_W=660
WINDOW_H=400
ICON_SIZE=128
APP_ICON_X=170
APP_ICON_Y=200
DROP_LINK_X=490
DROP_LINK_Y=200

log() { echo "[build-dmg] $*"; }
die() { echo >&2 "[build-dmg] error: $*"; exit 1; }

# Locate the single *.app inside APP_DIR (auto-updater invariant: exactly one).
app_bundle_name() {
	local count name
	count=$(find "$APP_DIR" -maxdepth 1 -name '*.app' | wc -l | tr -d '[:space:]')
	if [[ "$count" != "1" ]]; then
		echo >&2 "[build-dmg] expected exactly one *.app in $APP_DIR, found $count"
		return 1
	fi
	name=$(find "$APP_DIR" -maxdepth 1 -name '*.app' -exec basename {} \;)
	printf '%s' "$name"
}

# Copy just the single *.app bundle from APP_DIR into a clean staging dir.
# Staging keeps the create-dmg source folder free of the output DMG
# (DMG_DIR can equal APP_DIR) and of any stray Applications symlink left by a
# previous plain build.
stage_app() {
	local dest="$1" app_name
	app_name="$(app_bundle_name)" || return 1
	cp -R "$APP_DIR/$app_name" "$dest/$app_name"
}

build_styled_dmg() {
	local app_name background_tiff volicon tmp_dir stage_dir
	app_name="$(app_bundle_name)" || return 1
	tmp_dir="$(mktemp -d)"
	stage_dir="$(mktemp -d)"
	# shellcheck disable=SC2064
	trap "rm -rf '$tmp_dir' '$stage_dir'" RETURN

	stage_app "$stage_dir"

	# Combine @1x + @2x into a single HiDPI-tagged TIFF so the background
	# renders crisp on Retina displays instead of upscaled and blurry.
	background_tiff="$tmp_dir/dmg-background.tiff"
	log "building HiDPI background tiff"
	tiffutil -cathidpicheck "$BACKGROUND_1X" "$BACKGROUND_2X" -out "$background_tiff"

	# Derive the volume icon from the same iconset as the app icon.
	volicon="$tmp_dir/VolumeIcon.icns"
	log "building volume icon"
	iconutil -c icns "$ICONSET_DIR" -o "$volicon"

	rm -f "$OUTPUT_DMG"
	log "running create-dmg"
	"$CREATE_DMG" \
		--volname "$VOLNAME" \
		--volicon "$volicon" \
		--background "$background_tiff" \
		--window-pos 200 120 \
		--window-size "$WINDOW_W" "$WINDOW_H" \
		--icon-size "$ICON_SIZE" \
		--icon "$app_name" "$APP_ICON_X" "$APP_ICON_Y" \
		--hide-extension "$app_name" \
		--app-drop-link "$DROP_LINK_X" "$DROP_LINK_Y" \
		--hdiutil-quiet \
		--format UDZO \
		--filesystem "HFS+" \
		"$OUTPUT_DMG" \
		"$stage_dir"
	log "styled DMG written: $OUTPUT_DMG"
}

[[ -x "$CREATE_DMG" ]] || die "vendored create-dmg not executable at $CREATE_DMG"

build_styled_dmg
