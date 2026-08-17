#!/usr/bin/env bash
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

rm -f /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y ca-certificates curl gnupg ffmpeg tmux unzip jq
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --batch --yes --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg
printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu %s stable\n' "$(dpkg --print-architecture)" "$(. /etc/os-release && echo "$VERSION_CODENAME")" > /etc/apt/sources.list.d/docker.list
curl -fsSL https://deb.nodesource.com/setup_24.x | bash -
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin nodejs
systemctl enable --now docker
curl -fsSL -o /tmp/google-chrome.deb https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
apt-get install -y /tmp/google-chrome.deb
rm -f /tmp/google-chrome.deb
npm install -g agent-browser@latest @anthropic-ai/claude-code@latest @earendil-works/pi-coding-agent@latest
test -x /usr/bin/google-chrome
command -v agent-browser
