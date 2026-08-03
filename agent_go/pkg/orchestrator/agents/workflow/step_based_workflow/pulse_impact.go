package step_based_workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const pulseInterventionsSchema = `CREATE TABLE IF NOT EXISTS pulse_interventions (
	intervention_id TEXT PRIMARY KEY,
	pulse_run_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	criterion_id TEXT NOT NULL,
	impact_type TEXT NOT NULL,
	metric TEXT NOT NULL,
	expected_direction TEXT NOT NULL,
	scope_json TEXT NOT NULL DEFAULT '[]',
	provenance TEXT NOT NULL DEFAULT '',
	baseline_window TEXT NOT NULL DEFAULT '',
	checkpoint TEXT NOT NULL DEFAULT '',
	minimum_evidence_runs INTEGER NOT NULL DEFAULT 1,
	status TEXT NOT NULL DEFAULT 'awaiting_evidence',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`

const pulseInterventionSourcesSchema = `CREATE TABLE IF NOT EXISTS pulse_intervention_sources (
	intervention_id TEXT NOT NULL,
	source_type TEXT NOT NULL,
	source_id TEXT NOT NULL,
	PRIMARY KEY (intervention_id, source_type, source_id)
)`

const pulseGoalObservationsSchema = `CREATE TABLE IF NOT EXISTS pulse_goal_observations (
	observation_id TEXT PRIMARY KEY,
	criterion_id TEXT NOT NULL,
	metric TEXT NOT NULL,
	run_id TEXT NOT NULL,
	route TEXT NOT NULL DEFAULT '',
	environment TEXT NOT NULL DEFAULT '',
	value REAL,
	status TEXT NOT NULL DEFAULT '',
	unit TEXT NOT NULL DEFAULT '',
	observed_at TEXT NOT NULL,
	evidence_json TEXT NOT NULL DEFAULT '[]',
	recorded_at TEXT NOT NULL,
	UNIQUE (criterion_id, metric, run_id, route, environment)
)`

const pulseImpactAssessmentsSchema = `CREATE TABLE IF NOT EXISTS pulse_impact_assessments (
	assessment_id TEXT PRIMARY KEY,
	intervention_id TEXT NOT NULL,
	verdict TEXT NOT NULL,
	before_window TEXT NOT NULL,
	after_window TEXT NOT NULL,
	before_value REAL,
	after_value REAL,
	absolute_change REAL,
	relative_change REAL,
	confidence TEXT NOT NULL,
	confounders_json TEXT NOT NULL DEFAULT '[]',
	evidence_json TEXT NOT NULL DEFAULT '[]',
	next_checkpoint TEXT NOT NULL DEFAULT '',
	assessed_at TEXT NOT NULL
)`

type PulseInterventionSource struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}

type PulseIntervention struct {
	InterventionID      string                    `json:"intervention_id"`
	PulseRunID          string                    `json:"pulse_run_id,omitempty"`
	Title               string                    `json:"title"`
	CriterionID         string                    `json:"criterion_id"`
	ImpactType          string                    `json:"impact_type"`
	Metric              string                    `json:"metric"`
	ExpectedDirection   string                    `json:"expected_direction"`
	Scope               []string                  `json:"scope,omitempty"`
	Provenance          string                    `json:"provenance,omitempty"`
	BaselineWindow      string                    `json:"baseline_window,omitempty"`
	Checkpoint          string                    `json:"checkpoint,omitempty"`
	MinimumEvidenceRuns int                       `json:"minimum_evidence_runs"`
	Status              string                    `json:"status"`
	CreatedAt           string                    `json:"created_at,omitempty"`
	UpdatedAt           string                    `json:"updated_at,omitempty"`
	Sources             []PulseInterventionSource `json:"sources,omitempty"`
}

type PulseGoalObservation struct {
	ObservationID string   `json:"observation_id"`
	CriterionID   string   `json:"criterion_id"`
	Metric        string   `json:"metric"`
	RunID         string   `json:"run_id"`
	Route         string   `json:"route,omitempty"`
	Environment   string   `json:"environment,omitempty"`
	Value         *float64 `json:"value,omitempty"`
	Status        string   `json:"status,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	ObservedAt    string   `json:"observed_at"`
	Evidence      []string `json:"evidence,omitempty"`
	RecordedAt    string   `json:"recorded_at,omitempty"`
}

type PulseImpactAssessment struct {
	AssessmentID   string   `json:"assessment_id"`
	InterventionID string   `json:"intervention_id"`
	Verdict        string   `json:"verdict"`
	BeforeWindow   string   `json:"before_window"`
	AfterWindow    string   `json:"after_window"`
	BeforeValue    *float64 `json:"before_value,omitempty"`
	AfterValue     *float64 `json:"after_value,omitempty"`
	AbsoluteChange *float64 `json:"absolute_change,omitempty"`
	RelativeChange *float64 `json:"relative_change,omitempty"`
	Confidence     string   `json:"confidence"`
	Confounders    []string `json:"confounders,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
	NextCheckpoint string   `json:"next_checkpoint,omitempty"`
	AssessedAt     string   `json:"assessed_at"`
}

type PulseImpactUpdate struct {
	Interventions []PulseIntervention     `json:"interventions,omitempty"`
	Observations  []PulseGoalObservation  `json:"observations,omitempty"`
	Assessments   []PulseImpactAssessment `json:"assessments,omitempty"`
}

type PulseImpactLedger struct {
	Interventions []PulseIntervention     `json:"interventions"`
	Observations  []PulseGoalObservation  `json:"observations"`
	Assessments   []PulseImpactAssessment `json:"assessments"`
}

func ensurePulseImpactSchema(ctx context.Context, db pulseFindingLifecycleDB) error {
	for _, ddl := range []string{
		pulseInterventionsSchema,
		pulseInterventionSourcesSchema,
		pulseGoalObservationsSchema,
		pulseImpactAssessmentsSchema,
		`CREATE INDEX IF NOT EXISTS idx_pulse_interventions_criterion ON pulse_interventions(criterion_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_goal_observations_criterion ON pulse_goal_observations(criterion_id, metric, observed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_impact_assessments_intervention ON pulse_impact_assessments(intervention_id, assessed_at DESC)`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func pulseImpactID(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, strings.TrimSpace(part))
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\x00")))
	return hex.EncodeToString(sum[:])[:20]
}

func pulseImpactJSON(values []string) string {
	encoded, _ := json.Marshal(normalizedLifecycleStrings(values))
	return string(encoded)
}

func validPulseImpactType(value string) bool {
	switch value {
	case "direct_goal", "reliability", "measurement", "presentation_maintenance":
		return true
	default:
		return false
	}
}

func validPulseImpactVerdict(value string) bool {
	switch value {
	case "improved", "unchanged", "regressed", "inconclusive", "confounded":
		return true
	default:
		return false
	}
}

func validPulseInterventionStatus(value string) bool {
	switch value {
	case "awaiting_evidence", "measuring", "assessed", "retired":
		return true
	default:
		return false
	}
}

func validPulseImpactSourceType(value string) bool {
	switch value {
	case "attempt", "experiment", "finding", "review":
		return true
	default:
		return false
	}
}

// RecordPulseImpactUpdate stores the durable second half of the Pulse loop.
// Observations and assessments are append-only. Intervention definitions may
// advance their lifecycle status while retaining their stable identity.
func RecordPulseImpactUpdate(ctx context.Context, workspacePath string, update PulseImpactUpdate) (*PulseImpactLedger, error) {
	if len(update.Interventions) == 0 && len(update.Observations) == 0 && len(update.Assessments) == 0 {
		return nil, fmt.Errorf("at least one intervention, observation, or assessment is required")
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := ensurePulseImpactSchema(ctx, db); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	for _, intervention := range update.Interventions {
		intervention.InterventionID = strings.TrimSpace(intervention.InterventionID)
		intervention.Title = strings.TrimSpace(intervention.Title)
		intervention.CriterionID = strings.TrimSpace(intervention.CriterionID)
		intervention.ImpactType = strings.TrimSpace(intervention.ImpactType)
		intervention.Metric = strings.TrimSpace(intervention.Metric)
		intervention.ExpectedDirection = strings.TrimSpace(intervention.ExpectedDirection)
		if intervention.Title == "" || intervention.CriterionID == "" || intervention.Metric == "" {
			return nil, fmt.Errorf("each intervention requires title, criterion_id, and metric")
		}
		if !validPulseImpactType(intervention.ImpactType) {
			return nil, fmt.Errorf("invalid impact_type %q", intervention.ImpactType)
		}
		if intervention.ExpectedDirection != "increase" && intervention.ExpectedDirection != "decrease" && intervention.ExpectedDirection != "maintain" {
			return nil, fmt.Errorf("invalid expected_direction %q", intervention.ExpectedDirection)
		}
		if intervention.InterventionID == "" {
			intervention.InterventionID = "int-" + pulseImpactID(intervention.CriterionID, intervention.Metric, intervention.Title)
		}
		if intervention.MinimumEvidenceRuns < 1 {
			intervention.MinimumEvidenceRuns = 1
		}
		if strings.TrimSpace(intervention.Status) == "" {
			intervention.Status = "awaiting_evidence"
		}
		if !validPulseInterventionStatus(intervention.Status) {
			return nil, fmt.Errorf("invalid intervention status %q", intervention.Status)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pulse_interventions
			(intervention_id, pulse_run_id, title, criterion_id, impact_type, metric, expected_direction,
			 scope_json, provenance, baseline_window, checkpoint, minimum_evidence_runs, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(intervention_id) DO UPDATE SET
			 pulse_run_id=excluded.pulse_run_id, title=excluded.title, criterion_id=excluded.criterion_id,
			 impact_type=excluded.impact_type, metric=excluded.metric, expected_direction=excluded.expected_direction,
			 scope_json=excluded.scope_json, provenance=excluded.provenance,
			 baseline_window=excluded.baseline_window, checkpoint=excluded.checkpoint,
			 minimum_evidence_runs=excluded.minimum_evidence_runs, status=excluded.status, updated_at=excluded.updated_at`,
			intervention.InterventionID, strings.TrimSpace(intervention.PulseRunID), intervention.Title,
			intervention.CriterionID, intervention.ImpactType, intervention.Metric,
			intervention.ExpectedDirection, pulseImpactJSON(intervention.Scope), strings.TrimSpace(intervention.Provenance),
			strings.TrimSpace(intervention.BaselineWindow), strings.TrimSpace(intervention.Checkpoint),
			intervention.MinimumEvidenceRuns, strings.TrimSpace(intervention.Status), now, now); err != nil {
			return nil, err
		}
		for _, source := range intervention.Sources {
			source.SourceType = strings.TrimSpace(source.SourceType)
			source.SourceID = strings.TrimSpace(source.SourceID)
			if source.SourceType == "" || source.SourceID == "" {
				return nil, fmt.Errorf("intervention sources require source_type and source_id")
			}
			if !validPulseImpactSourceType(source.SourceType) {
				return nil, fmt.Errorf("invalid intervention source_type %q", source.SourceType)
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO pulse_intervention_sources
				(intervention_id, source_type, source_id) VALUES (?, ?, ?)`,
				intervention.InterventionID, strings.TrimSpace(source.SourceType), strings.TrimSpace(source.SourceID)); err != nil {
				return nil, err
			}
		}
	}

	for _, observation := range update.Observations {
		observation.CriterionID = strings.TrimSpace(observation.CriterionID)
		observation.Metric = strings.TrimSpace(observation.Metric)
		observation.RunID = strings.TrimSpace(observation.RunID)
		observation.ObservedAt = strings.TrimSpace(observation.ObservedAt)
		if observation.CriterionID == "" || observation.Metric == "" || observation.RunID == "" || observation.ObservedAt == "" {
			return nil, fmt.Errorf("each observation requires criterion_id, metric, run_id, and observed_at")
		}
		if observation.Value == nil && strings.TrimSpace(observation.Status) == "" {
			return nil, fmt.Errorf("observation %s/%s requires value or status", observation.CriterionID, observation.Metric)
		}
		observation.ObservationID = strings.TrimSpace(observation.ObservationID)
		if observation.ObservationID == "" {
			observation.ObservationID = "obs-" + pulseImpactID(observation.CriterionID, observation.Metric, observation.RunID, observation.Route, observation.Environment)
		}
		var value interface{}
		if observation.Value != nil {
			value = *observation.Value
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO pulse_goal_observations
			(observation_id, criterion_id, metric, run_id, route, environment, value, status, unit, observed_at, evidence_json, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			observation.ObservationID, observation.CriterionID, observation.Metric, observation.RunID,
			strings.TrimSpace(observation.Route), strings.TrimSpace(observation.Environment), value,
			strings.TrimSpace(observation.Status), strings.TrimSpace(observation.Unit), observation.ObservedAt,
			pulseImpactJSON(observation.Evidence), now); err != nil {
			return nil, err
		}
	}

	for _, assessment := range update.Assessments {
		assessment.InterventionID = strings.TrimSpace(assessment.InterventionID)
		assessment.Verdict = strings.TrimSpace(assessment.Verdict)
		assessment.BeforeWindow = strings.TrimSpace(assessment.BeforeWindow)
		assessment.AfterWindow = strings.TrimSpace(assessment.AfterWindow)
		assessment.Confidence = strings.TrimSpace(assessment.Confidence)
		assessment.AssessedAt = strings.TrimSpace(assessment.AssessedAt)
		if assessment.InterventionID == "" || assessment.BeforeWindow == "" || assessment.AfterWindow == "" || assessment.Confidence == "" || assessment.AssessedAt == "" {
			return nil, fmt.Errorf("each assessment requires intervention_id, windows, confidence, and assessed_at")
		}
		if !validPulseImpactVerdict(assessment.Verdict) {
			return nil, fmt.Errorf("invalid assessment verdict %q", assessment.Verdict)
		}
		if assessment.Confidence != "low" && assessment.Confidence != "medium" && assessment.Confidence != "high" {
			return nil, fmt.Errorf("invalid assessment confidence %q", assessment.Confidence)
		}
		var interventionExists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pulse_interventions WHERE intervention_id=?`, assessment.InterventionID).Scan(&interventionExists); err != nil {
			return nil, err
		}
		if interventionExists != 1 {
			return nil, fmt.Errorf("unknown intervention_id %q", assessment.InterventionID)
		}
		assessment.AssessmentID = strings.TrimSpace(assessment.AssessmentID)
		if assessment.AssessmentID == "" {
			assessment.AssessmentID = "impact-" + pulseImpactID(assessment.InterventionID, assessment.BeforeWindow, assessment.AfterWindow, assessment.AssessedAt)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO pulse_impact_assessments
			(assessment_id, intervention_id, verdict, before_window, after_window, before_value, after_value,
			 absolute_change, relative_change, confidence, confounders_json, evidence_json, next_checkpoint, assessed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			assessment.AssessmentID, assessment.InterventionID, assessment.Verdict,
			assessment.BeforeWindow, assessment.AfterWindow, nullableFloat(assessment.BeforeValue),
			nullableFloat(assessment.AfterValue), nullableFloat(assessment.AbsoluteChange), nullableFloat(assessment.RelativeChange),
			assessment.Confidence, pulseImpactJSON(assessment.Confounders), pulseImpactJSON(assessment.Evidence),
			strings.TrimSpace(assessment.NextCheckpoint), assessment.AssessedAt); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE pulse_interventions
			SET status='assessed', updated_at=? WHERE intervention_id=? AND status!='retired'`,
			now, assessment.InterventionID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return LoadPulseImpactLedger(ctx, workspacePath, 200)
}

func nullableFloat(value *float64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func decodePulseImpactStrings(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	if values == nil {
		return []string{}
	}
	return values
}

// LoadPulseImpactLedger returns current intervention definitions and immutable
// observation/assessment history, newest first. It is the source for Pulse UI
// and email projections; HTML is never the source of truth.
func LoadPulseImpactLedger(ctx context.Context, workspacePath string, limit int) (*PulseImpactLedger, error) {
	if limit <= 0 {
		limit = 200
	}
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return &PulseImpactLedger{Interventions: []PulseIntervention{}, Observations: []PulseGoalObservation{}, Assessments: []PulseImpactAssessment{}}, err
	}
	defer db.Close()
	if err := ensurePulseImpactSchema(ctx, db); err != nil {
		return nil, err
	}
	ledger := &PulseImpactLedger{Interventions: []PulseIntervention{}, Observations: []PulseGoalObservation{}, Assessments: []PulseImpactAssessment{}}

	rows, err := db.QueryContext(ctx, `SELECT intervention_id, pulse_run_id, title, criterion_id, impact_type, metric,
		expected_direction, scope_json, provenance, baseline_window, checkpoint, minimum_evidence_runs,
		status, created_at, updated_at FROM pulse_interventions ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item PulseIntervention
		var scopeJSON string
		if err := rows.Scan(&item.InterventionID, &item.PulseRunID, &item.Title, &item.CriterionID,
			&item.ImpactType, &item.Metric, &item.ExpectedDirection, &scopeJSON, &item.Provenance,
			&item.BaselineWindow, &item.Checkpoint, &item.MinimumEvidenceRuns, &item.Status,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		item.Scope = decodePulseImpactStrings(scopeJSON)
		sourceRows, sourceErr := db.QueryContext(ctx, `SELECT source_type, source_id FROM pulse_intervention_sources WHERE intervention_id=? ORDER BY source_type, source_id`, item.InterventionID)
		if sourceErr != nil {
			rows.Close()
			return nil, sourceErr
		}
		item.Sources = []PulseInterventionSource{}
		for sourceRows.Next() {
			var source PulseInterventionSource
			if err := sourceRows.Scan(&source.SourceType, &source.SourceID); err != nil {
				sourceRows.Close()
				rows.Close()
				return nil, err
			}
			item.Sources = append(item.Sources, source)
		}
		sourceRows.Close()
		ledger.Interventions = append(ledger.Interventions, item)
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT observation_id, criterion_id, metric, run_id, route, environment,
		value, status, unit, observed_at, evidence_json, recorded_at
		FROM pulse_goal_observations ORDER BY observed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item PulseGoalObservation
		var value sql.NullFloat64
		var evidenceJSON string
		if err := rows.Scan(&item.ObservationID, &item.CriterionID, &item.Metric, &item.RunID, &item.Route,
			&item.Environment, &value, &item.Status, &item.Unit, &item.ObservedAt, &evidenceJSON, &item.RecordedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if value.Valid {
			v := value.Float64
			item.Value = &v
		}
		item.Evidence = decodePulseImpactStrings(evidenceJSON)
		ledger.Observations = append(ledger.Observations, item)
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT assessment_id, intervention_id, verdict, before_window, after_window,
		before_value, after_value, absolute_change, relative_change, confidence, confounders_json,
		evidence_json, next_checkpoint, assessed_at FROM pulse_impact_assessments ORDER BY assessed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item PulseImpactAssessment
		var before, after, absolute, relative sql.NullFloat64
		var confoundersJSON, evidenceJSON string
		if err := rows.Scan(&item.AssessmentID, &item.InterventionID, &item.Verdict, &item.BeforeWindow,
			&item.AfterWindow, &before, &after, &absolute, &relative, &item.Confidence,
			&confoundersJSON, &evidenceJSON, &item.NextCheckpoint, &item.AssessedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if before.Valid {
			v := before.Float64
			item.BeforeValue = &v
		}
		if after.Valid {
			v := after.Float64
			item.AfterValue = &v
		}
		if absolute.Valid {
			v := absolute.Float64
			item.AbsoluteChange = &v
		}
		if relative.Valid {
			v := relative.Float64
			item.RelativeChange = &v
		}
		item.Confounders = decodePulseImpactStrings(confoundersJSON)
		item.Evidence = decodePulseImpactStrings(evidenceJSON)
		ledger.Assessments = append(ledger.Assessments, item)
	}
	rows.Close()
	return ledger, nil
}
