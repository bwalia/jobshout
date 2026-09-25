package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	BuiltinCourseGenerator = "course_generator"
	AgentNameCourse        = "Course Generator"
)

// Course run statuses.
const (
	CourseRunQueued    = "queued"
	CourseRunRunning   = "running"
	CourseRunCompleted = "completed"
	CourseRunFailed    = "failed"
	CourseRunCancelled = "cancelled"
)

// Course pipeline step keys, in order. A chapter's steps are reported on the
// run with the chapter position in the detail, so the step list stays fixed
// whatever the chapter count.
const (
	CourseStepResearching  = "researching"
	CourseStepOutlining    = "outlining"
	CourseStepWriting      = "writing"
	CourseStepReviewing    = "reviewing"
	CourseStepIllustrating = "illustrating"
	CourseStepQuizzing     = "quizzing"
	CourseStepSaved        = "saved"
)

// CourseStepOrder is the fixed step list shown on every run.
var CourseStepOrder = []string{
	CourseStepResearching,
	CourseStepOutlining,
	CourseStepWriting,
	CourseStepReviewing,
	CourseStepIllustrating,
	CourseStepQuizzing,
	CourseStepSaved,
}

// Course brief limits.
const (
	CourseMinChapters     = 1
	CourseMaxChapters     = 12
	CourseDefaultChapters = 5
	CourseDefaultLocale   = "en"
)

// CourseLevels mirrors the levels Workstation Academy accepts.
var CourseLevels = []string{"beginner", "intermediate", "advanced"}

// CourseBrief is what the user asked for. It is data, not code: a new course
// or a new language is a new brief, never a code change.
type CourseBrief struct {
	Topic        string   `json:"topic"`
	Audience     string   `json:"audience,omitempty"`
	Level        string   `json:"level"`
	ChapterCount int      `json:"chapter_count"`
	Locale       string   `json:"locale"`
	Context      string   `json:"context,omitempty"`
	SeedURLs     []string `json:"seed_urls,omitempty"`
	Focus        []string `json:"focus,omitempty"`
	Model        string   `json:"model,omitempty"`
}

// CreateCourseRunRequest is the launch payload for a Course Generator run.
type CreateCourseRunRequest struct {
	AgentID uuid.UUID   `json:"agent_id" validate:"required"`
	TaskID  *uuid.UUID  `json:"task_id"`
	Brief   CourseBrief `json:"brief"`
}

// CourseRunStep is one entry in the run's progress list.
type CourseRunStep struct {
	Key    string `json:"key"`
	Status string `json:"status"` // pending, running, done, failed, skipped
	Detail string `json:"detail,omitempty"`
}

// CourseOutline is the planned course: what the Academy course card shows,
// plus the chapter plan the writer works from.
type CourseOutline struct {
	Title            string                 `json:"title"`
	Description      string                 `json:"description"`
	Level            string                 `json:"level"`
	Category         string                 `json:"category"`
	Tags             []string               `json:"tags"`
	LearningOutcomes []string               `json:"learning_outcomes"`
	Chapters         []CourseOutlineChapter `json:"chapters"`
}

// CourseOutlineChapter is one planned chapter.
type CourseOutlineChapter struct {
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Objectives []string `json:"objectives"`
}

// CourseRun is one course generation.
type CourseRun struct {
	ID           uuid.UUID       `json:"id"`
	AgentID      uuid.UUID       `json:"agent_id"`
	TaskID       *uuid.UUID      `json:"task_id"`
	OrgID        uuid.UUID       `json:"org_id"`
	Status       string          `json:"status"`
	Brief        CourseBrief     `json:"brief"`
	Outline      *CourseOutline  `json:"outline,omitempty"`
	Steps        []CourseRunStep `json:"steps"`
	CoverURL     string          `json:"cover_url,omitempty"`
	Sources      []CourseSource  `json:"sources,omitempty"`
	Warnings     []string        `json:"warnings,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	RequestedBy  *uuid.UUID      `json:"requested_by"`
	HeartbeatAt  *time.Time      `json:"heartbeat_at,omitempty"`
	StartedAt    *time.Time      `json:"started_at"`
	CompletedAt  *time.Time      `json:"completed_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// CourseSource is a research source the course rests on.
type CourseSource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// CourseImage is a generated visual attached to a chapter.
type CourseImage struct {
	URL     string `json:"url"`
	Alt     string `json:"alt"`
	Caption string `json:"caption,omitempty"`
}

// CourseQuiz is a chapter's assessment. It is kept in JobShout only and is
// never rendered into lesson HTML, so answers cannot leak to learners.
type CourseQuiz struct {
	Questions []CourseQuizQuestion `json:"questions"`
}

// CourseQuizQuestion is a single multiple-choice question.
type CourseQuizQuestion struct {
	Question     string   `json:"question"`
	Options      []string `json:"options"`
	CorrectIndex int      `json:"correct_index"`
	Explanation  string   `json:"explanation"`
}

// CourseChapter is one generated chapter. One chapter becomes one Academy
// lesson. Locale is part of the key so a translation is a new row, not a
// schema change.
type CourseChapter struct {
	ID         uuid.UUID     `json:"id"`
	RunID      uuid.UUID     `json:"run_id"`
	Locale     string        `json:"locale"`
	Position   int           `json:"position"`
	Title      string        `json:"title"`
	Summary    string        `json:"summary"`
	Objectives []string      `json:"objectives"`
	Markdown   string        `json:"markdown"`
	HTML       string        `json:"html"`
	Images     []CourseImage `json:"images"`
	Quiz       *CourseQuiz   `json:"quiz,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}
