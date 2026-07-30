package main

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// week.go — the "This Week" view: combines the parent-configured recurring
// schedule (familyState.Schedule), the real activity-log history (see
// activity_log.go), and Veracross-derived deadlines (memory/school-
// deadlines.json, maintained by Pulse's school-portal check — see pulse.go)
// into one weekly grid, navigable to past weeks.

// SchoolDeadline is one assignment/test the agent found on the school
// portal. Written by the agent itself (via its shell, same as
// memory/browser-notes.md), fully rewritten each Pulse cycle — see the
// school-portal pulseCheck instruction in pulse.go.
type SchoolDeadline struct {
	Title   string `json:"title"`
	Subject string `json:"subject,omitempty"`
	DueDate string `json:"due_date,omitempty"` // "2026-08-03"
	Kind    string `json:"kind,omitempty"`     // "assignment" | "test"
}

type schoolDeadlinesFile struct {
	Deadlines []SchoolDeadline `json:"deadlines,omitempty"`
	UpdatedAt string           `json:"updated_at,omitempty"`
}

func loadSchoolDeadlines() []SchoolDeadline {
	abs, ok := resolveWorkspacePath("memory/school-deadlines.json")
	if !ok {
		return nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	var f schoolDeadlinesFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil
	}
	return f.Deadlines
}

// GET /api/child-schedule
func handleGetChildSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	writeJSON(w, http.StatusOK, s.Schedule)
}

type setChildScheduleRequest struct {
	Entries []ScheduleEntry `json:"entries"`
}

// POST /api/child-schedule — wholesale replace (the mini-editor always sends
// the whole edited list; conversational capture goes through
// set_child_schedule instead, which ADDS rather than replaces).
func handleSetChildSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req setChildScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	cleaned := make([]ScheduleEntry, 0, len(req.Entries))
	for _, e := range req.Entries {
		e.Day = strings.TrimSpace(e.Day)
		e.Start = strings.TrimSpace(e.Start)
		e.End = strings.TrimSpace(e.End)
		e.Label = strings.TrimSpace(e.Label)
		if e.Day == "" || e.Start == "" || e.End == "" || e.Label == "" {
			continue
		}
		cleaned = append(cleaned, e)
	}
	stateMu.Lock()
	s := loadState()
	s.Schedule.Entries = cleaned
	err := saveState(s)
	sched := s.Schedule
	stateMu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	mirrorChildSchedule(sched)
	writeJSON(w, http.StatusOK, sched)
}

type weekDay struct {
	Date       string             `json:"date"`    // "2026-07-28"
	Weekday    string             `json:"weekday"` // "Monday"
	Schedule   []ScheduleEntry    `json:"schedule,omitempty"`
	Activities []ActivityLogEntry `json:"activities,omitempty"`
	Deadlines  []SchoolDeadline   `json:"deadlines,omitempty"`
}

type weekResponse struct {
	WeekStart string           `json:"week_start"`
	WeekEnd   string           `json:"week_end"`
	Days      []weekDay        `json:"days"`
	Upcoming  []SchoolDeadline `json:"upcoming_deadlines,omitempty"`
}

// GET /api/week?offset=N — offset=0 is the current week (Monday-Sunday,
// local time), -1 is last week, 1 is next week, etc.
func handleGetWeek(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}

	now := time.Now()
	// Roll back to this week's Monday, then apply the offset in whole weeks.
	// time.Monday==1, time.Sunday==0 — normalize Sunday to 7 so the
	// subtraction below always lands on a Monday.
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -(weekday-1)+7*offset)
	monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())

	stateMu.Lock()
	s := loadState()
	stateMu.Unlock()
	entries := loadActivityLog()
	deadlines := loadSchoolDeadlines()

	days := make([]weekDay, 7)
	dateIndex := make(map[string]int, 7)
	for i := 0; i < 7; i++ {
		d := monday.AddDate(0, 0, i)
		dateStr := d.Format("2006-01-02")
		dateIndex[dateStr] = i
		days[i] = weekDay{Date: dateStr, Weekday: d.Weekday().String()}
		for _, se := range s.Schedule.Entries {
			if se.Day == d.Weekday().String() {
				days[i].Schedule = append(days[i].Schedule, se)
			}
		}
	}
	weekStart := days[0].Date
	weekEnd := days[6].Date

	for _, e := range entries {
		if i, ok := dateIndex[e.Date]; ok {
			days[i].Activities = append(days[i].Activities, e)
		}
	}

	var upcoming []SchoolDeadline
	for _, dl := range deadlines {
		if i, ok := dateIndex[dl.DueDate]; ok {
			days[i].Deadlines = append(days[i].Deadlines, dl)
		}
		if dl.DueDate >= weekStart && dl.DueDate <= weekEnd {
			upcoming = append(upcoming, dl)
		}
	}
	sort.Slice(upcoming, func(i, j int) bool { return upcoming[i].DueDate < upcoming[j].DueDate })

	writeJSON(w, http.StatusOK, weekResponse{
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
		Days:      days,
		Upcoming:  upcoming,
	})
}
