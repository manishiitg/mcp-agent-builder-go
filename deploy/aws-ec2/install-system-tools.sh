#!/usr/bin/env bash
# Install the CLIs that WORKFLOW shell commands need on the Video Studio /
# AgentWorks box, as root, through SSM.
#
# Why root and why SSM: workflow shells run under Landlock with a strict
# environment (HOME=/tmp, system PATH only) and can only read system roots
# (/usr, /bin, /lib, ...). A tool under the service user's ~/.local is
# invisible to them -- verified 2026-09-03: `aws: not found` inside the
# sandbox while it worked over SSH. So these go to /usr/local, which needs
# root; the box has no sudo and SSH is deploy-only, but the SSM agent is
# running and the instance role carries AmazonSSMManagedInstanceCore
# (instance-role.yaml), so `aws ssm send-command` is the admin channel.
#
# Idempotent: every step checks before installing. Re-run to add tools.
set -euo pipefail

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-RTS}"
AWS_REGION="${AWS_REGION:-us-west-2}"
STACK_NAME="${STACK_NAME:-video-studio-prod}"
NTN_VERSION_URL="${NTN_VERSION_URL:-https://ntn.dev}"

aws_rts() { aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION" "$@"; }
instance="$(aws_rts cloudformation describe-stacks --stack-name "$STACK_NAME" --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue|[0]' --output text)"

status="$(aws_rts ssm describe-instance-information --filters "Key=InstanceIds,Values=$instance" --query 'InstanceInformationList[0].PingStatus' --output text 2>/dev/null || true)"
if [[ "$status" != "Online" ]]; then
  echo "instance $instance is not an SSM managed instance yet (status: ${status:-none}); run attach-instance-role.sh and wait a few minutes" >&2
  exit 1
fi

# The remote script. Keep it plain sh-compatible bash; SSM runs it as root.
read -r -d '' REMOTE <<'REMOTE_SCRIPT' || true
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get install -y -q git unzip curl ca-certificates >/dev/null

# AWS CLI v2, system-wide. Credentials come from the instance role via IMDS.
if ! test -x /usr/local/bin/aws; then
  tmp="$(mktemp -d)"
  curl -fsS "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "$tmp/awscliv2.zip"
  (cd "$tmp" && unzip -q awscliv2.zip && ./aws/install -i /usr/local/aws-cli -b /usr/local/bin >/dev/null)
  rm -rf "$tmp"
fi

# Notion CLI (ntn): real binary under /usr/local/lib/ntn, launcher on PATH.
# The launcher maps the workflow-attached secret SECRET_NOTION_API_TOKEN (a
# global secret named NOTION_API_TOKEN) onto NOTION_API_TOKEN, which the CLI
# reads instead of a keychain; file-based auth, if ever used, goes to a
# writable dir because HOME is /tmp in the sandbox.
if ! test -x /usr/local/lib/ntn/ntn; then
  install -d -m 0755 /usr/local/lib/ntn
  NTN_INSTALL_DIR=/usr/local/lib/ntn bash -c 'curl -fsSL https://ntn.dev | bash' >/dev/null
fi
cat > /usr/local/bin/ntn <<'NTN'
#!/usr/bin/env bash
export NOTION_KEYRING="${NOTION_KEYRING:-0}"
export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-/tmp/.xdg-config}"
if [[ -z "${NOTION_API_TOKEN:-}" && -n "${SECRET_NOTION_API_TOKEN:-}" ]]; then export NOTION_API_TOKEN="$SECRET_NOTION_API_TOKEN"; fi
exec /usr/local/lib/ntn/ntn "$@"
NTN
chmod 0755 /usr/local/bin/ntn

echo "git: $(git --version)"
echo "aws: $(/usr/local/bin/aws --version)"
echo "ntn: $(/usr/local/bin/ntn --version)"
REMOTE_SCRIPT

cmd_id="$(aws_rts ssm send-command --instance-ids "$instance" --document-name AWS-RunShellScript --comment "install workflow system tools" --parameters "$(jq -cn --arg s "$REMOTE" '{commands: [$s], executionTimeout: ["900"]}')" --query 'Command.CommandId' --output text)"
echo "ssm command $cmd_id sent; waiting"
for _ in $(seq 1 90); do
  st="$(aws_rts ssm get-command-invocation --command-id "$cmd_id" --instance-id "$instance" --query Status --output text 2>/dev/null || echo Pending)"
  case "$st" in
    Success) aws_rts ssm get-command-invocation --command-id "$cmd_id" --instance-id "$instance" --query StandardOutputContent --output text; exit 0 ;;
    Failed|Cancelled|TimedOut) echo "ssm command $st" >&2; aws_rts ssm get-command-invocation --command-id "$cmd_id" --instance-id "$instance" --query '[StandardOutputContent,StandardErrorContent]' --output text | tail -n 40 >&2; exit 1 ;;
  esac
  sleep 10
done
echo "ssm command still running after 15 minutes: $cmd_id" >&2
exit 1
