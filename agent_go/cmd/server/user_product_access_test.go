package server

import (
	"net/http/httptest"
	"testing"
)

func withUserProductAccessFile(t *testing.T, content string) {
	t.Helper()
	workspace := &mockWorkspaceAPI{files: map[string]string{}}
	if content != "" {
		workspace.files[userProductAccessFilePath()] = content
	}
	server := httptest.NewServer(workspace)
	t.Cleanup(server.Close)
	t.Setenv("WORKSPACE_API_URL", server.URL)
}

func TestUserAllowedProductDefaultsToUnrestrictedWhenFileAbsent(t *testing.T) {
	withUserProductAccessFile(t, "")

	claims := &UserClaims{UserID: "u1", Username: "john"}
	if !userAllowedProduct(claims, "agentworks") {
		t.Fatal("user with no config entry should be unrestricted")
	}
	if !userAllowedWorkflowID(claims, "tectonicusadaytrading") {
		t.Fatal("user with no config entry should see every workflow")
	}
}

func TestUserAllowedProductRestrictsExplicitEntry(t *testing.T) {
	withUserProductAccessFile(t, `{
		"john": { "products": ["dominion"] },
		"manish": { "products": ["dominion", "agentworks"], "workflow_ids": ["tectonicusadaytrading"] }
	}`)

	john := &UserClaims{UserID: "u-john", Username: "john"}
	if !userAllowedProduct(john, "dominion") {
		t.Fatal("john should be allowed dominion")
	}
	if userAllowedProduct(john, "agentworks") {
		t.Fatal("john should not be allowed agentworks")
	}
	// john has no workflow_ids entry, so workflow access is unrestricted --
	// products and workflows are independent narrowings.
	if !userAllowedWorkflowID(john, "tectonicusadaytrading") {
		t.Fatal("john with no workflow_ids entry should be unrestricted for workflows")
	}

	manish := &UserClaims{UserID: "u-manish", Username: "manish"}
	if !userAllowedProduct(manish, "agentworks") {
		t.Fatal("manish should be allowed agentworks")
	}
	if !userAllowedWorkflowID(manish, "tectonicusadaytrading") {
		t.Fatal("manish should be allowed tectonicusadaytrading")
	}
	if userAllowedWorkflowID(manish, "some-other-workflow") {
		t.Fatal("manish should not be allowed a workflow outside his explicit list")
	}
}

func TestUserAllowedProductMatchesByUsernameCaseInsensitive(t *testing.T) {
	withUserProductAccessFile(t, `{ "John": { "products": ["dominion"] } }`)

	claims := &UserClaims{UserID: "u-john", Username: "john"}
	if userAllowedProduct(claims, "agentworks") {
		t.Fatal("normalized username match should still restrict")
	}
	if !userAllowedProduct(claims, "dominion") {
		t.Fatal("normalized username match should still allow the granted product")
	}
}

func TestFilterWorkflowManifestsForUserNarrowsList(t *testing.T) {
	withUserProductAccessFile(t, `{ "manish": { "products": ["dominion", "agentworks"], "workflow_ids": ["tectonicusadaytrading"] } }`)

	discovered := []DiscoveredWorkflow{
		{WorkspacePath: "Workflow/tectonicusadaytrading", Manifest: &WorkflowManifest{ID: "tectonicusadaytrading"}},
		{WorkspacePath: "Workflow/other", Manifest: &WorkflowManifest{ID: "other"}},
	}

	manish := &UserClaims{UserID: "u-manish", Username: "manish"}
	filtered := filterWorkflowManifestsForUser(manish, discovered)
	if len(filtered) != 1 || filtered[0].Manifest.ID != "tectonicusadaytrading" {
		t.Fatalf("filtered = %+v, want only tectonicusadaytrading", filtered)
	}

	unrestricted := &UserClaims{UserID: "u-other", Username: "someone-else"}
	if got := filterWorkflowManifestsForUser(unrestricted, discovered); len(got) != 2 {
		t.Fatalf("unrestricted user should see every workflow, got %d", len(got))
	}
}

func TestProductAccessResponseFieldsOmitsWhenUnrestricted(t *testing.T) {
	withUserProductAccessFile(t, "")

	claims := &UserClaims{UserID: "u1", Username: "someone"}
	fields := productAccessResponseFields(claims)
	if fields["allowed_products"] != nil {
		t.Fatalf("allowed_products = %v, want nil for unrestricted user", fields["allowed_products"])
	}
	if fields["allowed_workflow_ids"] != nil {
		t.Fatalf("allowed_workflow_ids = %v, want nil for unrestricted user", fields["allowed_workflow_ids"])
	}
}
