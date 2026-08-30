package step_based_workflow

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
)

func TestEffectiveBrowserModeForHostDownloadsResolvesAutoWithCDPPort(t *testing.T) {
	controller := &StepBasedWorkflowOrchestrator{}
	controller.SetBrowserMode("auto")
	controller.SetCdpPort(9222)

	if got := controller.effectiveBrowserModeForHostDownloads(); got != "cdp" {
		t.Fatalf("effective browser mode = %q, want cdp", got)
	}
}

func TestEffectiveBrowserModeForHostDownloadsDoesNotTreatAutoAsCDPWithoutPort(t *testing.T) {
	controller := &StepBasedWorkflowOrchestrator{}
	controller.SetBrowserMode("auto")

	if got := controller.effectiveBrowserModeForHostDownloads(); got != "" {
		t.Fatalf("effective browser mode = %q, want empty without a CDP port", got)
	}
}

func TestAutoCDPWorkshopGuardIncludesReadWriteHostDownloads(t *testing.T) {
	hostDownloads := t.TempDir()
	t.Setenv("PI_HOST_DOWNLOADS_PATH", "")
	t.Setenv("HOST_DOWNLOADS_PATH", hostDownloads)
	controller := &StepBasedWorkflowOrchestrator{}
	controller.SetBrowserMode("auto")
	controller.SetCdpPort(9222)

	reads, writes := controller.appendCDPHostDownloadsPaths([]string{"Workflow/jobsearch"}, []string{"Workflow/jobsearch"})
	if !containsString(reads, hostDownloads) {
		t.Fatalf("auto-CDP read paths omit host Downloads: %v", reads)
	}
	if !containsString(writes, hostDownloads) {
		t.Fatalf("auto-CDP write paths omit host Downloads: %v", writes)
	}

	sessionID := "auto-cdp-workshop-downloads-test"
	t.Cleanup(func() { common.ClearSessionShellConfig(sessionID) })
	common.SetSessionFolderGuard(sessionID, reads, []string{"Workflow/jobsearch"})
	common.SetSessionFolderGuardBlockedWritePaths(sessionID, []string{hostDownloads})
	controller.grantSessionCDPHostDownloadsReadWrite(sessionID)
	guard := common.GetSessionShellConfig(sessionID)
	if guard == nil || !containsString(guard.ReadPaths, hostDownloads) || !containsString(guard.WritePaths, hostDownloads) || containsString(guard.BlockedWritePaths, hostDownloads) {
		t.Fatalf("host Downloads must be read-write and not write-blocked: %#v", guard)
	}
}
