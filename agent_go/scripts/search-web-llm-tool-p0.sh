#!/usr/bin/env bash
# Agentic P0 for search_web_llm. Capture records real provider receipts; verify
# enforces that an agent reviewed every receipt before release.
set -euo pipefail

cd "$(dirname "$0")/.."
review_dir="$(pwd)/cmd/server/virtual-tools/testdata/search-web-llm-agent-reviews"

capture() {
  MLP_AGENT_REVIEW_CAPTURE=1 \
  MLP_AGENT_REVIEW_DIR="$review_dir" \
  go test -tags search_web_llm_tool_p0_live ./cmd/server/virtual-tools \
    -run TestSearchWebLLMToolP0Live -count=1 -timeout 90m
}

verify() {
  SEARCH_WEB_LLM_TOOL_P0_VERIFY=1 \
  go test ./cmd/server/virtual-tools -run TestSearchWebLLMToolP0ReviewsApproved -count=1
}

pi_bridge() {
  RUN_SEARCH_WEB_LLM_PI_BRIDGE_P0=1 \
  MLP_AGENT_REVIEW_CAPTURE=1 \
  MLP_AGENT_REVIEW_DIR="$review_dir" \
  go test -tags search_web_llm_pi_bridge_p0_live ./cmd/server/virtual-tools \
    -run TestSearchWebLLMPiBridgeP0 -count=1 -timeout 15m
}

case "${1:-all}" in
  capture) capture ;;
  verify) verify ;;
  pi) pi_bridge ;;
  all) capture; verify ;;
  *) echo "usage: $0 [capture|verify|pi|all]"; exit 2 ;;
esac
