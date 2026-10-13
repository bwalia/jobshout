package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	BuiltinVideoProducer = "video_producer"
	AgentNameVideo       = "Video Producer"
)

// Video run statuses. scripting → queued → rendering → done; any of the
// first three can end in failed or cancelled.
const (
	VideoRunScripting = "scripting"
	VideoRunQueued    = "queued"
	VideoRunRendering = "rendering"
	VideoRunDone      = "done"
	VideoRunFailed    = "failed"
	VideoRunCancelled = "cancelled"
)

// VideoShot is one line of narration and the picture shown while it is spoken.
type VideoShot struct {
	Narration string `json:"narration"`
	Visual    string `json:"visual"`
}

// VideoBrief is what a launch asked for.
type VideoBrief struct {
	Seconds  int    `json:"seconds"`
	Format   string `json:"format"`
	Voice    string `json:"voice"`
	Style    string `json:"style,omitempty"`
	Motion   bool   `json:"motion"`
	Captions bool   `json:"captions"`
	Notes    string `json:"notes,omitempty"`
}

// VideoRun is one Video Producer launch and where it has got to. Stage and
// Message are the workstation's own words while it renders ("motion",
// "Animating shot 4 of 10"); before that they say the script is being written
// or the render is waiting its turn.
type VideoRun struct {
	ID              uuid.UUID   `json:"id"`
	OrgID           uuid.UUID   `json:"org_id"`
	TaskID          *uuid.UUID  `json:"task_id,omitempty"`
	AgentID         *uuid.UUID  `json:"agent_id,omitempty"`
	RequestedBy     *uuid.UUID  `json:"requested_by,omitempty"`
	Topic           string      `json:"topic"`
	Brief           VideoBrief  `json:"brief"`
	Title           string      `json:"title"`
	Shots           []VideoShot `json:"shots"`
	Status          string      `json:"status"`
	Stage           string      `json:"stage"`
	Progress        float64     `json:"progress"`
	Message         string      `json:"message"`
	Error           string      `json:"error,omitempty"`
	Notes           []string    `json:"notes"`
	VideoURL        string      `json:"video_url,omitempty"`
	DurationSeconds float64     `json:"duration_seconds"`
	MotionModel     string      `json:"motion_model,omitempty"`
	EstimateMinutes int         `json:"estimate_minutes"`
	CreatedAt       time.Time   `json:"created_at"`
	StartedAt       *time.Time  `json:"started_at,omitempty"`
	FinishedAt      *time.Time  `json:"finished_at,omitempty"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// Unfinished reports whether the run can still change.
func (r VideoRun) Unfinished() bool {
	return r.Status == VideoRunScripting || r.Status == VideoRunQueued || r.Status == VideoRunRendering
}
