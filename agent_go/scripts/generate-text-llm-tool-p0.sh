#!/usr/bin/env bash
# Durable agentic P0 for generate_text_llm. Capture exercises Pi-dev through
# mcpagent's real bridge at low, medium, and high tiers; verify requires an
# agent-reviewed live receipt.
set -euo pipefail

cd "$(dirname "$0")/.."
review_dir="$(pwd)/cmd/server/virtual-tools/testdata/generate-text-llm-agent-reviews"

capture() {
  if ! command -v pi >/dev/null; then
    echo "Pi CLI is required for the generate_text_llm agentic P0" >&2
    exit 1
  fi
  if [[ -z "${PI_API_KEY:-}" && -z "${GEMINI_API_KEY:-}" ]]; then
    echo "PI_API_KEY or GEMINI_API_KEY is required for the generate_text_llm agentic P0" >&2
    exit 1
  fi
  RUN_GENERATE_TEXT_LLM_PI_BRIDGE_P0=1 \
  MLP_AGENT_REVIEW_CAPTURE=1 \
  MLP_AGENT_REVIEW_DIR="$review_dir" \
  go test -tags generate_text_llm_pi_bridge_p0_live ./cmd/server/virtual-tools \
    -run TestGenerateTextLLMPiBridgeP0 -count=1 -timeout 30m
}

verify() {
  GENERATE_TEXT_LLM_TOOL_P0_VERIFY=1 \
  go test ./cmd/server/virtual-tools -run TestGenerateTextLLMToolP0ReviewApproved -count=1
}

case "${1:-all}" in
  capture) capture ;;
  verify) verify ;;
  all) capture; verify ;;
  *) echo "usage: $0 [capture|verify|all]"; exit 2 ;;
esac
