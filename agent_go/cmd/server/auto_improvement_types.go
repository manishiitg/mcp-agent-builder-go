package server

// =====================================================================
// Auto-Improvement Framework — shared types
// Schemas: schemas/auto-improvement.schema.json
// Doc:     docs/workflow/auto_improvement_framework.md
// =====================================================================

// OversightMode — per-workflow oversight policy for high-risk framework
// changes.
type OversightMode string

const (
	OversightManual     OversightMode = "manual"
	OversightSupervised OversightMode = "supervised"
	OversightAutonomous OversightMode = "autonomous"
)
