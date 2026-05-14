#!/bin/bash
set -e

GIT_REMOTE_URL=$(git remote get-url origin)
GIT_AUTHOR_EMAIL=$(git log -1 --pretty=format:'%ae')
GIT_TAG=$(git describe --tags --dirty)
GIT_HEAD=$(git rev-parse HEAD)
BLUE_RELEASE_TAR="${BLUE_RELEASE_TAR:-rune-agent.tar.gz}"
: "${BLUECTL_CONFIG_DIR:?BLUECTL_CONFIG_DIR is not set. Use the rune-agent-{prod,staging}-dist* make targets so the bluectl env+os+arch is selected by the target.}"
BLUE_EXEC=(bluectl -c "$BLUECTL_CONFIG_DIR")
# BLUE_TARGET_OS / BLUE_TARGET_ARCH let cross-arch make targets stamp the
# manifest with the artifact's real target rather than the host's hardware.
OS="${BLUE_TARGET_OS:-$(uname | awk '{print tolower($0)}')}"
if [[ -n "$BLUE_TARGET_ARCH" ]]; then
	ARCH="$BLUE_TARGET_ARCH"
elif [[ "$(uname)" == "Darwin" ]] && [[ "$(sysctl -n hw.optional.arm64 2>/dev/null)" == "1" ]]; then
	ARCH="arm64"
else
	ARCH=$(uname -m)
fi
BLUE_RELEASE_TAG="$GIT_TAG"

echo "Pushing tarball for OS '$OS' and arch '$ARCH'";

if [[ -z "${BLUE_PGP_KEY}" ]]; then
    echo "BLUE_PGP_KEY is not set. See bluectl release upload -h for help."
	exit 1;
fi

if [[ -z "${BLUE_PGP_KEYRING}" ]]; then
    echo "BLUE_PGP_KEYRING is not set. See bluectl release upload -h for help."
	exit 1;
fi

blue_release_dist() {
	GIT_LOG=$(git log --pretty=format:"%h: %s" $GIT_LOG_RANGE)
	printf "\n$GIT_LOG\n";

	echo "uploading $BLUE_RELEASE_TAG"
	"${BLUE_EXEC[@]}" release upload \
		-d target-os=$OS \
		-d target-arch=$ARCH \
		-d git-remote-url=$GIT_REMOTE_URL \
		-d git-author-email=$GIT_AUTHOR_EMAIL \
		-d git-tag=$GIT_TAG -d git-head=$GIT_HEAD \
		-d git-log="$GIT_LOG" \
		-y \
		-k $BLUE_PGP_KEY \
		-r $BLUE_PGP_KEYRING rune-agent $BLUE_RELEASE_TAG $BLUE_RELEASE_TAR
}

# check if HEAD is tagged; if not, use annotate with range between latest tag and HEAD
git describe --contains 2>&1 1> /dev/null;
if [ $? -ne 0 ];
then
	GIT_LOG_RANGE="$(git tag -l --sort=-version:refname | head -1)...HEAD";
	printf "using git range between latest tag and latest tag + added commits: $GIT_LOG_RANGE:";
	blue_release_dist;
else
	GIT_LOG_RANGE="$(git tag -l --sort=-version:refname | head -2 | xargs | sed 's/ /.../g')";
	printf "using git range between latest tags: $GIT_LOG_RANGE:";
	blue_release_dist;
fi
