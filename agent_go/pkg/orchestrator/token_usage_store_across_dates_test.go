package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestReadRunAcrossDatesMergesOnlyRequestedRunOncePerShard(t *testing.T) {
	const workspace = "/workspace/Workflow/test"
	run := "iteration-0/group-1"
	dir := filepath.Join(workspace, "costs", "execution", "group-1")
	day := func(date string, requested, other int) string {
		file := DailyGroupTokenUsageFile{
			Date: date,
			RunFolders: map[string]*TokenUsageFile{
				run: {
					ByModel:        map[string]*ModelTokenUsage{"claude-opus-5": {InputTokens: requested, TotalCost: float64(requested) / 100}},
					ByStepAndModel: map[string]map[string]*ModelTokenUsage{"execute:step-a": {"claude-opus-5": {InputTokens: requested, TotalCost: float64(requested) / 100}}},
				},
				"iteration-1/group-1": {ByModel: map[string]*ModelTokenUsage{"claude-opus-5": {InputTokens: other}}},
			},
		}
		data, err := json.Marshal(file)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	files := map[string]string{
		filepath.Join(dir, "2026-08-02.json"): day("2026-08-02", 100, 900),
		filepath.Join(dir, "2026-08-03.json"): day("2026-08-03", 250, 800),
	}
	store := &tokenUsageFileStore{
		workspacePath: workspace,
		readFile: func(_ context.Context, path string) (string, error) {
			if content, ok := files[path]; ok {
				return content, nil
			}
			return "", fmt.Errorf("not found")
		},
		listFiles: func(_ context.Context, path string) ([]string, error) {
			if path != dir {
				t.Fatalf("list path = %q, want %q", path, dir)
			}
			return []string{"notes.txt", "2026-08-03.json", "2026-08-02.json"}, nil
		},
		writeFile:  func(context.Context, string, string) error { return nil },
		deleteFile: func(context.Context, string) error { return nil },
		warnf:      func(string) {},
		now:        func() time.Time { return time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC) },
	}

	got := store.readRunAcrossDates(context.Background(), run)
	if got.ByModel["claude-opus-5"].InputTokens != 350 {
		t.Fatalf("merged model input = %d, want 350", got.ByModel["claude-opus-5"].InputTokens)
	}
	if got.ByStepAndModel["execute:step-a"]["claude-opus-5"].InputTokens != 350 {
		t.Fatalf("merged step input = %d, want 350", got.ByStepAndModel["execute:step-a"]["claude-opus-5"].InputTokens)
	}
	if got.ByModel["claude-opus-5"].InputTokens == 350+1700 {
		t.Fatal("query included another run from the same group shards")
	}
}
