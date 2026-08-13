package step_based_workflow

import (
	"runtime/debug"
	"strings"
	"sync"
)

// PLAT-072. A finding records what it observed but not what it observed it
// against, so nothing distinguishes "still true" from "was true in July".
//
// The cost of that is measured: six workflows reported a cost-attribution
// defect between 2026-07-29 and 2026-08-06 that commit 0f6519640 fixed on
// 2026-08-06. Because the findings carried no version context, they still read
// as current, and a reviewer holding a full day of context on this codebase
// filed a fresh P1 for the completed work. Deciding staleness required reading
// each finding and recalling when the fix landed — a memory test, not a control.
//
// Stamping the platform revision at first observation makes that mechanical: a
// finding first seen before the commit that fixed it is a closure candidate,
// and one seen after is not.

// injectedPlatformVersion is set at link time by run_server_with_logging.sh:
//
//	-ldflags "-X <this package>.injectedPlatformVersion=<rev>"
//
// It exists because Go's own VCS stamping is unavailable here: a go.work above
// the repo puts every build in workspace mode, which disables -buildvcs
// silently — even an explicit -buildvcs=true yields no vcs.* settings. Verified
// against a real `go build ./cmd/server`, so the build-info path below is a
// fallback for other build modes rather than the primary source.
var injectedPlatformVersion string

var (
	platformVersionOnce  sync.Once
	platformVersionValue string
)

// PlatformVersion returns the VCS revision this binary was built from, or an
// empty string when it cannot be determined.
//
// Empty is a legitimate answer and callers must treat it as "unknown", never as
// "old". A `go build` without VCS stamping, a dirty tree, or a module built
// outside a repository all produce it. A sweep that treated unknown as stale
// would close live findings, which is the failure this whole mechanism exists
// to prevent — so unknown must always fall back to reading the finding.
//
// The value is resolved once: build info is immutable for the process lifetime,
// and this is called on every concern write.
func PlatformVersion() string {
	platformVersionOnce.Do(func() {
		if injected := strings.TrimSpace(injectedPlatformVersion); injected != "" {
			platformVersionValue = injected
			return
		}
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		revision, dirty := "", false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = strings.TrimSpace(setting.Value)
			case "vcs.modified":
				dirty = setting.Value == "true"
			}
		}
		if revision == "" {
			return
		}
		if len(revision) > 12 {
			revision = revision[:12]
		}
		// A dirty tree is marked rather than dropped. The revision still locates
		// the finding in history, and the suffix stops a sweep from treating an
		// uncommitted build as an exact match for that commit's state.
		if dirty {
			revision += "+dirty"
		}
		platformVersionValue = revision
	})
	return platformVersionValue
}
