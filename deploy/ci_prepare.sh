#!/bin/sh
set -e
set -x

mkdir -p ~/.ssh
chmod 700 ~/.ssh
printf '%s\n' "$GIT_SSH_KEY" | tr -d '\r' > ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa
ssh-keyscan -t rsa git.unstable.build >> ~/.ssh/known_hosts
ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts
git config --global url.ssh://git@github.com/.insteadOf https://github.com/
