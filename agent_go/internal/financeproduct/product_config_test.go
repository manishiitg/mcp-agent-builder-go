package financeproduct

import (
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func TestFinanceManifestDeclaresProjectScopeAndNarrowAllowlist(t *testing.T) {
	manifest, err := FinanceManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Profile.ID != "finance" {
		t.Fatalf("unexpected profile id: %q", manifest.Profile.ID)
	}
	// Project, not global, despite spanning multiple sources -- discovered
	// empirically (2026-08-16), not analogized from Chief of Staff: global
	// scope makes provider_options non-authoritative (the user's own
	// selection always wins, by design) and takes the dynamic delegation
	// prompt instead of this profile's own prompt.file. Both defeat the
	// point of this profile. See the product.yaml comment.
	if manifest.Profile.Scope != agentprofiles.ProfileScopeProject {
		t.Fatalf("finance must declare scope: project, got %q", manifest.Profile.Scope)
	}
	if manifest.Profile.Runtime.Provider != "claude-code" || manifest.Profile.Runtime.ModelID != "claude-sonnet-5" {
		t.Fatalf("expected provider=claude-code model_id=claude-sonnet-5, got provider=%q model_id=%q", manifest.Profile.Runtime.Provider, manifest.Profile.Runtime.ModelID)
	}
	// Kept for debuggability (turn-based, easier to reason about than a
	// long-lived tmux session), NOT for security -- corrected 2026-08-17.
	// A codex-cli finance chat once made 12 tool calls outside the
	// registered set before this was set, but a live re-test under the
	// ACTUAL security boundary (agent_tools.mode: mcp_only, below) with
	// transport: auto (native/tmux) restored found those 12 calls all
	// resolve through the same tool_policy-gated registry via codex's
	// code-execution-mode bridge -- the original leak almost certainly
	// traced to this profile still being scope: global at the time (see
	// the product.yaml comment), not to transport. See the product.yaml
	// comment for the full correction.
	if manifest.Profile.Runtime.Transport != "structured" {
		t.Fatalf("finance must declare runtime.transport: structured, got transport=%q", manifest.Profile.Runtime.Transport)
	}
	if !strings.EqualFold(manifest.Profile.Runtime.AgentTools.Mode, "mcp_only") {
		t.Fatalf("finance must declare agent_tools.mode: mcp_only explicitly, got %q", manifest.Profile.Runtime.AgentTools.Mode)
	}
	// A second tool was reached for live (2026-08-16), independent of
	// transport: structured: `nodeRepl`/`js`, with working fetch and a
	// real filesystem cwd -- this is docs/bugs/hybrid_profile_told_it_
	// has_no_shell.md's section 4 ("personal MCP servers leak into
	// product sessions"), a developer's own ~/.codex/config.toml leaking
	// in, not codex-cli's own intrinsic toolset -- very likely a
	// local-dev-machine artifact, not something a real deployed user
	// would hit. Curated to exactly one verified provider anyway, since
	// that doesn't retroactively prove codex-cli or cursor-cli safe under
	// a real allowlist for real financial data. See the product.yaml
	// comment for what "verified" actually means here: Finance's own
	// direct live test (two clean turns under this exact transport +
	// agent_tools combination), NOT inferred from Video Studio's
	// reliance -- Video Studio's own claude-code sessions run under a
	// different transport (transport: auto resolves to native/tmux for
	// claude-code, confirmed directly 2026-08-17), so its production
	// usage doesn't actually validate this specific combination.
	if len(manifest.Profile.Runtime.ProviderOptions) != 1 || manifest.Profile.Runtime.ProviderOptions[0].Provider != "claude-code" {
		t.Fatalf("finance must curate runtime.provider_options to exactly claude-code, got %+v", manifest.Profile.Runtime.ProviderOptions)
	}
	// This is the load-bearing property this profile exists for: a real,
	// server-enforced allowlist naming only the one query tool -- narrower
	// than a workflow's own run mode, whose "no plan changes" is prompt-text
	// only (see docs/design/workflow_custom_ui_product.md Gap 1).
	if !manifest.Profile.ToolPolicy.IsAllowlist() {
		t.Fatal("finance must declare tool_policy.mode: allowlist -- an unrestricted chat over financial data is exactly the gap this profile exists to close")
	}
	// execute_shell_command is intentionally included -- it's the platform's
	// only call path to a custom tool (get_api_spec discovery + curl to
	// $MCP_CUSTOM/<tool>), the same pattern Video Studio's product.yaml
	// documents. See the product.yaml comment for why this isn't "general
	// shell access" in practice, and prompts/system-prompt.md for how the
	// model is told to scope its use.
	wantEnabled := map[string]bool{"query_finance_source": false, "execute_shell_command": false}
	for _, name := range manifest.Profile.ToolPolicy.Enabled {
		if _, expected := wantEnabled[name]; !expected {
			t.Fatalf("unexpected tool in allowlist: %q -- finance chat should have no write, no delegation, no schedule, no workflow-execution tools beyond query_finance_source and its call path", name)
		}
		wantEnabled[name] = true
	}
	for name, found := range wantEnabled {
		if !found {
			t.Fatalf("expected tool_policy.enabled to include %q", name)
		}
	}
	wantTools := map[string]bool{"finance.query-source": false}
	for _, binding := range manifest.Profile.Tools {
		if _, expected := wantTools[binding.ID]; !expected {
			t.Fatalf("unexpected tool binding %q", binding.ID)
		}
		wantTools[binding.ID] = true
	}
	for id, found := range wantTools {
		if !found {
			t.Fatalf("expected profile.tools[] to bind %q -- without this, registerAgentProfileTools registers nothing for a finance chat even though the ToolFactory exists", id)
		}
	}
	if manifest.UI.Surface != "finance" {
		t.Fatalf("unexpected ui.surface: %q", manifest.UI.Surface)
	}
	if len(manifest.Workflows.Enabled) != 0 {
		t.Fatalf("finance has no fixed pipelines; workflows.enabled should stay empty: %v", manifest.Workflows.Enabled)
	}
}

func TestBuiltinAgentProfileValidatesAndResolvesProjectScope(t *testing.T) {
	profile := BuiltinAgentProfile()
	if err := agentprofiles.Validate(profile); err != nil {
		t.Fatalf("built-in profile failed validation: %v", err)
	}
	if profile.EffectiveScope() != agentprofiles.ProfileScopeProject {
		t.Fatalf("EffectiveScope() = %q, want project", profile.EffectiveScope())
	}
	if profile.SystemPromptTemplate == "" {
		t.Fatal("expected a non-empty rendered system prompt template")
	}
	if !profile.BuiltIn {
		t.Fatal("expected BuiltIn to be true")
	}
}

func TestBuiltinAgentProfilesReturnsExactlyOneVersion(t *testing.T) {
	profiles := BuiltinAgentProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected exactly one built-in profile, got %d", len(profiles))
	}
	if profiles[0].Version != 1 {
		t.Fatalf("expected version 1, got %d", profiles[0].Version)
	}
}

func TestRenderPromptSucceedsAgainstAPromptContext(t *testing.T) {
	// Unlike Chief of Staff's placeholder prompt (global scope, never
	// actually sent to the model), this profile's project scope means
	// prompts/system-prompt.md IS the live prompt a finance chat sees --
	// confirmed live (2026-08-16) that an earlier global-scoped version of
	// this profile silently never sent it at all. This render must succeed
	// and its content actually matters now.
	profile := BuiltinAgentProfile()
	rendered, err := agentprofiles.RenderPrompt(profile, agentprofiles.PromptContext{
		ProjectTitle:  "Finance",
		LocalDateTime: "Monday, 1 January 2026 at 9:00 AM UTC",
	})
	if err != nil {
		t.Fatalf("RenderPrompt failed: %v", err)
	}
	if rendered == "" {
		t.Fatal("expected non-empty rendered prompt")
	}
}

func TestRegisterAgentProfileRuntimeRegistersTheQueryToolFactory(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	if err := RegisterAgentProfileRuntime(registry, "http://127.0.0.1:0"); err != nil {
		t.Fatalf("RegisterAgentProfileRuntime failed: %v", err)
	}
	for _, profile := range BuiltinAgentProfiles() {
		if err := registry.RegisterProfile(profile); err != nil {
			t.Fatalf("RegisterProfile failed: %v", err)
		}
	}
	tool, err := registry.BuildTool(agentprofiles.ToolBinding{ID: "finance.query-source"}, agentprofiles.ToolRuntimeContext{UserID: "u1", SessionID: "s1"})
	if err != nil {
		t.Fatalf("BuildTool failed: %v", err)
	}
	if tool.Name != "query_finance_source" {
		t.Fatalf("unexpected tool name: %q", tool.Name)
	}
}

func TestFinanceSourceDBPathsMatchTheFrontendAdapters(t *testing.T) {
	// The five paths here are duplicated by hand in
	// frontend/src/products/finance/adapters/*.ts (plain string constants,
	// not shared logic -- see finance_query_tool.go's own comment). This
	// pins the Go side's exact set so a source can't silently go missing or
	// get renamed without a test failing.
	want := map[string]string{
		"hdfc":        "Workflow/HDFC-Personal-Accounts/db/db.sqlite",
		"icici":       "Workflow/ICICI-BANK-PARSING/db/db.sqlite",
		"mutual_fund": "Workflow/Mututal-Fund/db/db.sqlite",
		"tax":         "Workflow/check-form-26as-xspaces/db/db.sqlite",
		"gst":         "Workflow/gstdatacollection/db/db.sqlite",
	}
	if len(financeSourceDBPaths) != len(want) {
		t.Fatalf("expected %d sources, got %d: %v", len(want), len(financeSourceDBPaths), financeSourceDBPaths)
	}
	for source, path := range want {
		if got := financeSourceDBPaths[source]; got != path {
			t.Fatalf("source %q: got path %q, want %q", source, got, path)
		}
	}
}
