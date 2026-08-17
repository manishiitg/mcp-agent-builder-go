# Video Studio — Linux / EC2 Deployment Runbook

This is the isolated public Video Studio deployment.

- URL: `https://video.realtrainingsys.com`
- Region: `us-west-2`
- Stack: `video-studio-prod`
- Deploy method: local build → `rsync` over restricted SSH → user-level systemd restart
- Infrastructure: CloudFormation

The deployer uses the `RTS` AWS profile and an SSH key. SSH ingress is limited
to the deployer's current public IP; it is not publicly open.

## Deploy

From the repository root:

```bash
AWS_PROFILE_NAME=RTS \
AWS_REGION=us-west-2 \
SSH_KEY_PATH=/Users/mipl/.ssh/id_ed25519 \
bash deploy/aws-ec2/deploy-rootless.sh
```

This release path connects as `video-studio`, writes only its own application
directory, and restarts only its user services. It neither runs `sudo` nor
touches infrastructure, the shared login password, or unrelated services.

`deploy-aws-ec2.sh` is retained only as the original bootstrap installer; do
not use it for normal releases.

## Runtime security model

Normal releases run with no `sudo`. A one-time bootstrap/migration uses
administrator access to create the dedicated account and its initial service
files; the running product and every subsequent release use `video-studio`.

| Component | Runtime identity |
| --- | --- |
| Agent API | `video-studio` system user |
| Workspace API | `video-studio` system user |
| Cookie-auth gateway | `video-studio` system user |
| HTTPS proxy | Existing Caddy container, UID/GID `10001` (`video-studio`) |
| Frontend files | Served by the non-root gateway from the release directory |

The release directory and runtime environment are owned by `video-studio` at
`/var/lib/video-studio/video-studio`. The user cannot read the legacy
root-owned environment file under `/opt`.

Do not set `WorkingDirectory` to a release path that the runtime user cannot
traverse. Do not run an installer with `RELEASE_DIR=/opt/video-studio/current`:
that can make the `current` symlink point to itself. Always use a concrete path
under `/opt/video-studio/releases/`.

## Global secrets

Deployment-managed global secrets are stored in AWS Secrets Manager:

```text
video-studio/global-secrets
```

The secret value is a JSON object with upper-snake-case keys:

```json
{
  "EXAMPLE_API_KEY": "replace-me"
}
```

During deployment, each entry becomes `GLOBAL_SECRET_<NAME>` in the
runtime environment file. The agent makes it available to every Video
Studio project as `SECRET_<NAME>`. Values are never returned by the UI or
`list_secrets`; only names are visible.

`CLAUDE_CODE_OAUTH_TOKEN` is the sole reserved entry. It is a Claude Code
setup token used as the deployment-wide provider default, not a project tool
secret. A saved project-specific Claude Code token overrides it.

The deploy script retrieves the Secrets Manager value into a temporary mode
`0600` file, uploads it, installs it, then removes the temporary upload. Never
put actual secret values in this repository, a release directory, or command
output. `deploy/aws-ec2/global-secrets.env` is intentionally gitignored as an
emergency local fallback; use the included `.example` file only as a format
reference.

## MCP configuration

This product has its own intentionally empty MCP config:

```text
/var/lib/video-studio/video-studio/current/configs/mcp_servers_video_studio.json
```

Do not copy the generic developer workstation MCP catalog into this host.
Product tools are provided through the bundled `mcpbridge` binary.

## Post-deploy checks

Run these checks from a machine with the deploy SSH key. They show service
status and secret names only—never secret values.

```bash
ssh -i /Users/mipl/.ssh/id_ed25519 video-studio@44.253.29.127 \
  'systemctl --user is-active video-studio-agent video-studio-workspace video-studio-gateway'

ssh -i /Users/mipl/.ssh/id_ed25519 video-studio@44.253.29.127 \
  'awk -F= "/^GLOBAL_SECRET_/ {print \$1}" /var/lib/video-studio/video-studio/.env | sort'

curl -fsSI https://video.realtrainingsys.com/
```

An unauthenticated HTTP `303` redirect to `/login` is expected.

## Browser automation prerequisite

The bootstrap template and repair script install both Google Chrome and the
`agent-browser` CLI. The rootless runtime uses:

```text
AGENT_BROWSER_EXECUTABLE_PATH=/usr/bin/google-chrome
```

This path must remain available to the guarded workspace process; it is not
interchangeable with `/usr/bin/chromium`. Every normal rootless deploy checks
for both `/usr/bin/google-chrome` and `agent-browser` before it uploads or
restarts anything. If either check fails, run the administrator-only bootstrap
or repair script first, then return to the normal SSH + rsync deploy path.

The Linux shell sandbox is Landlock-first with a verified mount-namespace
fallback. Do not run the service as root or grant it `CAP_SYS_ADMIN`.

## Future SSM option

The current deployment uses SSH + rsync. If it later moves to SSM, use an S3
or CI-built artifact for file delivery and SSM only for controlled remote
installation. The EC2 instance role—not the deployer user—will need
`AmazonSSMManagedInstanceCore`.
