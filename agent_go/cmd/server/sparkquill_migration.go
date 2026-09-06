package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/sparkquillproduct"
)

// sparkQuillKnownEngines lists the parent profile's declared provider option
// ids — the only engines a migrated family.json may name.
func sparkQuillKnownEngines() []string {
	for _, p := range sparkquillproduct.BuiltinAgentProfiles() {
		if p.ID != sparkquillproduct.ParentProfileID {
			continue
		}
		ids := make([]string, 0, len(p.Runtime.ProviderOptions))
		for _, o := range p.Runtime.ProviderOptions {
			ids = append(ids, o.ID)
		}
		return ids
	}
	return nil
}

// runSparkQuillLegacyMigrationIfNeeded imports a standalone SparkQuill home
// (~/.sunlit-learning) on a desktop install's first boot. Desktop only: the
// hosted server has no legacy home to read and never migrates. Non-fatal by
// design — with no marker written, a failed attempt simply retries next boot,
// and a non-empty unmarked workspace is left alone with instructions.
func runSparkQuillLegacyMigrationIfNeeded() {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("NATIVE_WORKSPACE")), "true") || IsMultiUserMode() {
		return
	}
	if strings.TrimSpace(os.Getenv("SPARKQUILL_SKIP_MIGRATION")) != "" {
		return
	}
	docsDir := strings.TrimSpace(os.Getenv("WORKSPACE_DOCS_PATH"))
	if docsDir == "" {
		return
	}
	source := sparkquillproduct.DefaultLegacySourceDir()
	report, err := sparkquillproduct.MigrateLegacy(sparkquillproduct.LegacyMigrationOptions{
		SourceDir:    source,
		DocsDir:      docsDir,
		UserID:       GetDefaultUserID(),
		KnownEngines: sparkQuillKnownEngines(),
		Log:          func(format string, args ...interface{}) { log.Printf("[sparkquill-migrate] "+format, args...) },
	})
	switch {
	case errors.Is(err, sparkquillproduct.ErrTargetNotEmpty):
		log.Printf("[sparkquill-migrate] %v — run `agent-server server migrate-sparkquill --allow-existing` to merge %s into it", err, source)
		return
	case err != nil:
		log.Printf("[sparkquill-migrate] failed (will retry next start): %v", err)
		return
	case report.Skipped != "":
		return
	}
	seedSparkQuillCheckinState(docsDir, GetDefaultUserID(), source)
}

// seedSparkQuillCheckinState mirrors the standalone app's check-in switch
// onto the platform schedule, stamped as just-run so the first platform boot
// does not fire a check-in in the middle of onboarding (productschedule's
// cadence rule runs immediately on an empty last_run_at).
func seedSparkQuillCheckinState(docsDir, userID, sourceDir string) {
	enabled, _ := sparkquillproduct.LegacyPulseEnabled(sourceDir)
	if !enabled {
		return
	}
	statePath := filepath.Join(docsDir, filepath.FromSlash(productScheduleStatePath(userID)))
	all := map[string]productScheduleUserState{}
	if b, err := os.ReadFile(statePath); err == nil { //nolint:gosec // G304: path is derived from the server's own docs root.
		_ = json.Unmarshal(b, &all)
	}
	key := productScheduleStateKey(sparkquillproduct.ParentProfileID, "pulse")
	if _, ok := all[key]; ok {
		return
	}
	on := true
	all[key] = productScheduleUserState{Enabled: &on, LastRunAt: time.Now().UTC().Format(time.RFC3339)}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		log.Printf("[sparkquill-migrate] could not seed check-in state: %v", err)
		return
	}
	if err := os.WriteFile(statePath, append(data, '\n'), 0o600); err != nil {
		log.Printf("[sparkquill-migrate] could not seed check-in state: %v", err)
		return
	}
	log.Printf("[sparkquill-migrate] check-in was on in the legacy app; enabled on the platform, next run after the usual cadence")
}

var migrateSparkQuillCmd = &cobra.Command{
	Use:   "migrate-sparkquill",
	Short: "Copy a standalone SparkQuill home (~/.sunlit-learning) into the platform family workspace",
	Long: `Copies a pre-platform SparkQuill install (~/.sunlit-learning, the old standalone server's home) into <docs-dir>/_users/<user>/Chats/SparkQuill.
The source is never modified. A marker in the target makes the copy idempotent;
a non-empty target without that marker is refused unless --allow-existing is
given, in which case files are merged and never overwritten.`,
	RunE: runMigrateSparkQuill,
}

func init() {
	migrateSparkQuillCmd.Flags().String("from", sparkquillproduct.DefaultLegacySourceDir(), "Standalone SparkQuill home to read")
	migrateSparkQuillCmd.Flags().String("docs-dir", os.Getenv("WORKSPACE_DOCS_PATH"), "Platform workspace root (defaults to WORKSPACE_DOCS_PATH)")
	migrateSparkQuillCmd.Flags().String("user", "", "Platform user id (defaults to the single-user default)")
	migrateSparkQuillCmd.Flags().Bool("dry-run", false, "Plan and report without writing")
	migrateSparkQuillCmd.Flags().Bool("allow-existing", false, "Merge into a non-empty target (never overwriting)")
}

func runMigrateSparkQuill(cmd *cobra.Command, _ []string) error {
	source, _ := cmd.Flags().GetString("from")
	docsDir, _ := cmd.Flags().GetString("docs-dir")
	userID, _ := cmd.Flags().GetString("user")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	allowExisting, _ := cmd.Flags().GetBool("allow-existing")
	if strings.TrimSpace(docsDir) == "" {
		return fmt.Errorf("--docs-dir (or WORKSPACE_DOCS_PATH) is required")
	}
	if strings.TrimSpace(userID) == "" {
		userID = GetDefaultUserID()
	}
	report, err := sparkquillproduct.MigrateLegacy(sparkquillproduct.LegacyMigrationOptions{
		SourceDir:     source,
		DocsDir:       docsDir,
		UserID:        userID,
		DryRun:        dryRun,
		AllowExisting: allowExisting,
		KnownEngines:  sparkQuillKnownEngines(),
		Log:           func(format string, args ...interface{}) { fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...) },
	})
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(out))
	if err != nil {
		return err
	}
	if !dryRun && report.Marker != "" {
		seedSparkQuillCheckinState(docsDir, userID, source)
	}
	return nil
}
