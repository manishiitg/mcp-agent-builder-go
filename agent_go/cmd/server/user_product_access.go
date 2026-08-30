package server

import (
	"context"
	"encoding/json"
	"strings"
)

// Per-user product/workflow access grants, layered on top of the existing
// per-workflow read/write/owner grants in workflow_permissions.go. A user
// with NO entry here is unrestricted -- this file only ever narrows access
// for users explicitly listed, so every other deployment (desktop,
// single-user servers, multi-user servers that never write this file) keeps
// today's unrestricted behavior unchanged.
//
// Storage: config/user-product-access.json, shape:
//
//	{
//	  "manish": { "products": ["dominion", "agentworks"], "workflow_ids": ["tectonicusadaytrading"] },
//	  "john":   { "products": ["dominion"] }
//	}
//
// Keys are normalized (lowercase, trimmed) and matched against UserID,
// Username, or Email, same precedence and normalization as
// workflowAccessForIdentity in workflow_permissions.go. There is no admin
// CRUD for this file yet -- it's written once via the workspace API during
// deployment; add upsert/delete handlers here if the user list grows enough
// to need one.

type UserProductAccess struct {
	Products    []string `json:"products,omitempty"`
	WorkflowIDs []string `json:"workflow_ids,omitempty"`
}

func userProductAccessFilePath() string {
	return "config/user-product-access.json"
}

func readUserProductAccessFile() (map[string]UserProductAccess, error) {
	data, exists, err := readFileFromWorkspace(context.Background(), userProductAccessFilePath())
	if err != nil {
		return nil, err
	}
	if !exists {
		return map[string]UserProductAccess{}, nil
	}
	var raw map[string]UserProductAccess
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil, err
	}
	out := make(map[string]UserProductAccess, len(raw))
	for k, v := range raw {
		key := normalizeWorkflowPermissionKey(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out, nil
}

func userProductAccessForIdentity(userID, username, email string) (UserProductAccess, bool) {
	entries, err := readUserProductAccessFile()
	if err != nil || len(entries) == 0 {
		return UserProductAccess{}, false
	}
	for _, key := range []string{userID, username, email} {
		normalized := normalizeWorkflowPermissionKey(key)
		if normalized == "" {
			continue
		}
		if access, ok := entries[normalized]; ok {
			return access, true
		}
	}
	return UserProductAccess{}, false
}

func userProductAccessForClaims(claims *UserClaims) (UserProductAccess, bool) {
	if claims == nil {
		return UserProductAccess{}, false
	}
	return userProductAccessForIdentity(claims.UserID, claims.Username, claims.Email)
}

// userAllowedProduct reports whether claims may use the given product
// surface. A user with no explicit entry, or an entry with an empty
// Products list, is unrestricted. An empty product name (the generic,
// profile-less AgentWorks chat path has no product) is always allowed here
// -- workflow-level access is what actually gates that path, via
// userAllowedWorkflowID.
func userAllowedProduct(claims *UserClaims, product string) bool {
	product = strings.TrimSpace(product)
	if product == "" {
		return true
	}
	access, ok := userProductAccessForClaims(claims)
	if !ok || len(access.Products) == 0 {
		return true
	}
	for _, allowed := range access.Products {
		if strings.EqualFold(strings.TrimSpace(allowed), product) {
			return true
		}
	}
	return false
}

// userAllowedWorkflowID reports whether claims may see/run the given
// workflow. A user with no explicit entry, or an entry with an empty
// WorkflowIDs list, is unrestricted.
func userAllowedWorkflowID(claims *UserClaims, workflowID string) bool {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return true
	}
	access, ok := userProductAccessForClaims(claims)
	if !ok || len(access.WorkflowIDs) == 0 {
		return true
	}
	for _, allowed := range access.WorkflowIDs {
		if strings.EqualFold(strings.TrimSpace(allowed), workflowID) {
			return true
		}
	}
	return false
}

// filterWorkflowManifestsForUser narrows a discovered-workflows list to the
// ones claims is allowed to see. An unrestricted user (no entry, or an entry
// with an empty WorkflowIDs list) gets the list back unchanged, including a
// nil input turning into a nil/empty output rather than panicking.
func filterWorkflowManifestsForUser(claims *UserClaims, discovered []DiscoveredWorkflow) []DiscoveredWorkflow {
	access, ok := userProductAccessForClaims(claims)
	if !ok || len(access.WorkflowIDs) == 0 {
		return discovered
	}
	filtered := make([]DiscoveredWorkflow, 0, len(discovered))
	for _, workflow := range discovered {
		if workflow.Manifest == nil {
			continue
		}
		if userAllowedWorkflowID(claims, workflow.Manifest.ID) {
			filtered = append(filtered, workflow)
		}
	}
	return filtered
}

// productAccessResponseFields is merged into GET /api/auth/me the same way
// workflowPermissionResponseFields already is. allowed_products/
// allowed_workflow_ids are omitted (null) when the user has no explicit
// entry, meaning "unrestricted" -- the frontend treats null the same as an
// empty allowlist would be wrong to treat it as.
func productAccessResponseFields(claims *UserClaims) map[string]interface{} {
	access, ok := userProductAccessForClaims(claims)
	out := map[string]interface{}{
		"allowed_products":     nil,
		"allowed_workflow_ids": nil,
	}
	if ok && len(access.Products) > 0 {
		out["allowed_products"] = access.Products
	}
	if ok && len(access.WorkflowIDs) > 0 {
		out["allowed_workflow_ids"] = access.WorkflowIDs
	}
	return out
}
