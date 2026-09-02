package model

import (
	"time"

	"github.com/google/uuid"
)

const AgentNameCareerOps = "CareerOps"

// Application tracker states. Transitions are enforced in package career.
const (
	CareerStatusEvaluated = "evaluated"
	CareerStatusApplied   = "applied"
	CareerStatusResponded = "responded"
	CareerStatusInterview = "interview"
	CareerStatusOffer     = "offer"
	CareerStatusRejected  = "rejected"
	CareerStatusDiscarded = "discarded"
	CareerStatusSkip      = "skip"
	CareerStatusHired     = "hired"
)

const (
	CareerPipelineOpen        = "open"
	CareerPipelineClosed      = "closed"
	CareerPipelineBlacklisted = "blacklisted"
)

const (
	CareerEvalModeFull   = "full"
	CareerEvalModeTriage = "triage"
)

const (
	CareerArtifactCV      = "cv"
	CareerArtifactCover   = "cover"
	CareerArtifactEmail   = "email"
	CareerArtifactAnswers = "answers"
)

const (
	CareerStoryCV      = "cv"
	CareerStoryUser    = "user_stated"
	CareerStoryDerived = "derived_unverified"
)

// RecommendApplyMin is the default floor for "apply". House rules may raise it,
// never lower it below this without an explicit profile override.
const RecommendApplyMin = 4.0

// RecommendFormAnswersMin is the floor for drafting Block H application answers.
const RecommendFormAnswersMin = 4.5

type CareerIdentity struct {
	FullName string   `json:"full_name,omitempty"`
	Email    string   `json:"email,omitempty"`
	Phone    string   `json:"phone,omitempty"`
	Links    []string `json:"links,omitempty"`
}

type CareerTargets struct {
	Titles     []string `json:"titles,omitempty"`
	Seniority  string   `json:"seniority,omitempty"`
	Industries []string `json:"industries,omitempty"`
	MinComp    string   `json:"min_comp,omitempty"`
}

type CareerLocation struct {
	Cities     []string `json:"cities,omitempty"`
	Remote     bool     `json:"remote,omitempty"`
	Relocation bool     `json:"relocation,omitempty"`
}

type CareerWorkAuth struct {
	Countries         []string `json:"countries,omitempty"`
	NeedsSponsorship  bool     `json:"needs_sponsorship,omitempty"`
	AuthorizedAlready bool     `json:"authorized_already,omitempty"`
}

type CareerProfile struct {
	ID          uuid.UUID      `json:"id"`
	OrgID       uuid.UUID      `json:"org_id"`
	UserID      uuid.UUID      `json:"user_id"`
	CVMarkdown  string         `json:"cv_markdown"`
	Identity    CareerIdentity `json:"identity"`
	Targets     CareerTargets  `json:"targets"`
	Location    CareerLocation `json:"location"`
	WorkAuth    CareerWorkAuth `json:"work_auth"`
	Voice       string         `json:"voice"`
	HouseRules  string         `json:"house_rules"`
	ProofPoints string         `json:"proof_points"`
	Narrative   string         `json:"narrative"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type UpdateCareerProfileRequest struct {
	CVMarkdown  *string         `json:"cv_markdown,omitempty"`
	Identity    *CareerIdentity `json:"identity,omitempty"`
	Targets     *CareerTargets  `json:"targets,omitempty"`
	Location    *CareerLocation `json:"location,omitempty"`
	WorkAuth    *CareerWorkAuth `json:"work_auth,omitempty"`
	Voice       *string         `json:"voice,omitempty"`
	HouseRules  *string         `json:"house_rules,omitempty"`
	ProofPoints *string         `json:"proof_points,omitempty"`
	Narrative   *string         `json:"narrative,omitempty"`
}

type CareerPipelineItem struct {
	ID                uuid.UUID  `json:"id"`
	OrgID             uuid.UUID  `json:"org_id"`
	ProfileID         uuid.UUID  `json:"profile_id"`
	ListingURL        string     `json:"listing_url"`
	Company           string     `json:"company"`
	Title             string     `json:"title"`
	Source            string     `json:"source"`
	Status            string     `json:"status"`
	Liveness          string     `json:"liveness"`
	LivenessCheckedAt *time.Time `json:"liveness_checked_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type CareerApplication struct {
	ID         uuid.UUID `json:"id"`
	OrgID      uuid.UUID `json:"org_id"`
	ProfileID  uuid.UUID `json:"profile_id"`
	Company    string    `json:"company"`
	Role       string    `json:"role"`
	ListingURL string    `json:"listing_url"`
	Status     string    `json:"status"`
	Score      *float64  `json:"score,omitempty"`
	Via        string    `json:"via"`
	Agency     string    `json:"agency"`
	Employer   string    `json:"employer"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CareerStatusEvent struct {
	ID            uuid.UUID `json:"id"`
	OrgID         uuid.UUID `json:"org_id"`
	ProfileID     uuid.UUID `json:"profile_id"`
	ApplicationID uuid.UUID `json:"application_id"`
	FromStatus    string    `json:"from_status"`
	ToStatus      string    `json:"to_status"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

// CareerScore is holistic 1–5 across five dimensions — not a formula of them.
// Block G (legitimacy) is stored on the evaluation, never mixed into Overall.
type CareerScore struct {
	Overall              float64            `json:"overall"`
	Dimensions           map[string]float64 `json:"dimensions,omitempty"`
	RecommendApply       bool               `json:"recommend_apply"`
	RecommendFormAnswers bool               `json:"recommend_form_answers"`
	Recommendation       string             `json:"recommendation,omitempty"`
}

type CareerEvalBlocks struct {
	A        string `json:"a,omitempty"`
	B        string `json:"b,omitempty"`
	C        string `json:"c,omitempty"`
	D        string `json:"d,omitempty"`
	E        string `json:"e,omitempty"`
	F        string `json:"f,omitempty"`
	G        string `json:"g,omitempty"`
	H        string `json:"h,omitempty"`
	WorkAuth string `json:"work_auth,omitempty"`
}

type CareerEvaluation struct {
	ID             uuid.UUID        `json:"id"`
	OrgID          uuid.UUID        `json:"org_id"`
	ProfileID      uuid.UUID        `json:"profile_id"`
	ApplicationID  *uuid.UUID       `json:"application_id,omitempty"`
	PipelineItemID *uuid.UUID       `json:"pipeline_item_id,omitempty"`
	ListingURL     string           `json:"listing_url"`
	JDText         string           `json:"jd_text"`
	Company        string           `json:"company"`
	Role           string           `json:"role"`
	Blocks         CareerEvalBlocks `json:"blocks"`
	Score          CareerScore      `json:"score"`
	ReportMarkdown string           `json:"report_markdown"`
	LegitimacyTier string           `json:"legitimacy_tier"`
	HardStop       bool             `json:"hard_stop"`
	HardStopReason string           `json:"hard_stop_reason"`
	Mode           string           `json:"mode"`
	CreatedAt      time.Time        `json:"created_at"`
}

type CareerArtifact struct {
	ID            uuid.UUID  `json:"id"`
	OrgID         uuid.UUID  `json:"org_id"`
	ProfileID     uuid.UUID  `json:"profile_id"`
	ApplicationID *uuid.UUID `json:"application_id,omitempty"`
	EvaluationID  *uuid.UUID `json:"evaluation_id,omitempty"`
	Kind          string     `json:"kind"`
	Title         string     `json:"title"`
	BodyMarkdown  string     `json:"body_markdown"`
	FileID        string     `json:"file_id,omitempty"`
	HasPDF        bool       `json:"has_pdf"`
	CreatedAt     time.Time  `json:"created_at"`
}

type CareerBlacklistEntry struct {
	ID        uuid.UUID `json:"id"`
	OrgID     uuid.UUID `json:"org_id"`
	ProfileID uuid.UUID `json:"profile_id"`
	Company   string    `json:"company"`
	Domain    string    `json:"domain"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type CareerPortal struct {
	ID           uuid.UUID `json:"id"`
	OrgID        uuid.UUID `json:"org_id"`
	ProfileID    uuid.UUID `json:"profile_id"`
	Board        string    `json:"board"`
	Slug         string    `json:"slug"`
	Company      string    `json:"company"`
	TitleInclude []string  `json:"title_include"`
	TitleExclude []string  `json:"title_exclude"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CareerStory struct {
	ID         uuid.UUID `json:"id"`
	OrgID      uuid.UUID `json:"org_id"`
	ProfileID  uuid.UUID `json:"profile_id"`
	Title      string    `json:"title"`
	Situation  string    `json:"situation"`
	Task       string    `json:"task"`
	Action     string    `json:"action"`
	Result     string    `json:"result"`
	Reflection string    `json:"reflection"`
	Provenance string    `json:"provenance"`
	Tags       []string  `json:"tags"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type CareerScanRun struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	ProfileID    uuid.UUID  `json:"profile_id"`
	Status       string     `json:"status"`
	Added        int        `json:"added"`
	Skipped      int        `json:"skipped"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type CareerDoctorReport struct {
	OK       bool     `json:"ok"`
	Warnings []string `json:"warnings"`
	Info     []string `json:"info"`
}

type CareerIntakeProposal struct {
	Summary string                     `json:"summary"`
	Patch   UpdateCareerProfileRequest `json:"patch"`
}

type EvaluateCareerRequest struct {
	JobURL           string `json:"job_url,omitempty"`
	JDText           string `json:"jd_text,omitempty"`
	Mode             string `json:"mode,omitempty"` // full | triage
	TailorCV         bool   `json:"tailor_cv,omitempty"`
	ConfirmBlacklist bool   `json:"confirm_blacklist,omitempty"`
}

type CareerEvaluateResult struct {
	Evaluation   *CareerEvaluation     `json:"evaluation"`
	Application  *CareerApplication    `json:"application,omitempty"`
	BlacklistHit *CareerBlacklistEntry `json:"blacklist_hit,omitempty"`
	Dead         bool                  `json:"dead,omitempty"`
	DeadReason   string                `json:"dead_reason,omitempty"`
	Artifacts    []CareerArtifact      `json:"artifacts,omitempty"`
}

type ScanCareerRequest struct {
	Board   string `json:"board,omitempty"` // greenhouse | ashby | lever
	Slug    string `json:"slug,omitempty"`
	Company string `json:"company,omitempty"`
	Query   string `json:"query,omitempty"` // extra title include
}

type CareerScanResult struct {
	Run   *CareerScanRun       `json:"run"`
	Added []CareerPipelineItem `json:"added"`
}

type SetCareerStatusRequest struct {
	Status string `json:"status" validate:"required"`
	Note   string `json:"note,omitempty"`
}

type AddCareerBlacklistRequest struct {
	Company string `json:"company,omitempty"`
	Domain  string `json:"domain,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type AddCareerPortalRequest struct {
	Board        string   `json:"board"`
	Slug         string   `json:"slug"`
	Company      string   `json:"company,omitempty"`
	TitleInclude []string `json:"title_include,omitempty"`
	TitleExclude []string `json:"title_exclude,omitempty"`
}

type CareerContact struct {
	ID            uuid.UUID  `json:"id"`
	OrgID         uuid.UUID  `json:"org_id"`
	ProfileID     uuid.UUID  `json:"profile_id"`
	ApplicationID *uuid.UUID `json:"application_id,omitempty"`
	Name          string     `json:"name"`
	Role          string     `json:"role"`
	Company       string     `json:"company"`
	// Email is third-party PII. Never log it.
	Email         string    `json:"email,omitempty"`
	LinkedInURL   string    `json:"linkedin_url,omitempty"`
	Note          string    `json:"note,omitempty"`
	LinkedInDraft string    `json:"linkedin_draft,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type AddCareerContactRequest struct {
	ApplicationID *uuid.UUID `json:"application_id,omitempty"`
	Name          string     `json:"name"`
	Role          string     `json:"role,omitempty"`
	Company       string     `json:"company,omitempty"`
	Email         string     `json:"email,omitempty"`
	LinkedInURL   string     `json:"linkedin_url,omitempty"`
	Note          string     `json:"note,omitempty"`
}

type CareerFollowup struct {
	ID            uuid.UUID `json:"id"`
	OrgID         uuid.UUID `json:"org_id"`
	ProfileID     uuid.UUID `json:"profile_id"`
	ApplicationID uuid.UUID `json:"application_id"`
	DueAt         time.Time `json:"due_at"`
	Kind          string    `json:"kind"`
	Draft         string    `json:"draft"`
	Sent          bool      `json:"sent"`
	CreatedAt     time.Time `json:"created_at"`
}

type CareerPatterns struct {
	Applications int            `json:"applications"`
	ByStatus     map[string]int `json:"by_status"`
	AvgScore     float64        `json:"avg_score"`
	SkillGaps    []string       `json:"skill_gaps,omitempty"`
}

type CareerInterviewPrep struct {
	Company        string        `json:"company"`
	Role           string        `json:"role"`
	Status         string        `json:"status,omitempty"`
	ScoreFloorMet  bool          `json:"score_floor_met"`
	Stories        []CareerStory `json:"stories"`
	PrepMarkdown   string        `json:"prep_markdown"`
	NeverSubmit    bool          `json:"never_submit"`
	NotLegalAdvice bool          `json:"not_legal_advice"`
}

type CareerOfferPrep struct {
	Company        string `json:"company"`
	Role           string `json:"role"`
	PrepMarkdown   string `json:"prep_markdown"`
	NotLegalAdvice bool   `json:"not_legal_advice"`
}

type CareerSalaryGap struct {
	Desired        string `json:"desired"`
	Advertised     string `json:"advertised"`
	Actual         string `json:"actual"`
	Note           string `json:"note"`
	NotLegalAdvice bool   `json:"not_legal_advice"`
}

type CareerSalaryObservation struct {
	ID            uuid.UUID  `json:"id"`
	OrgID         uuid.UUID  `json:"org_id"`
	ProfileID     uuid.UUID  `json:"profile_id"`
	ApplicationID *uuid.UUID `json:"application_id,omitempty"`
	Desired       string     `json:"desired"`
	Advertised    string     `json:"advertised"`
	Actual        string     `json:"actual"`
	CreatedAt     time.Time  `json:"created_at"`
}

type CareerOffer struct {
	ID            uuid.UUID      `json:"id"`
	OrgID         uuid.UUID      `json:"org_id"`
	ProfileID     uuid.UUID      `json:"profile_id"`
	ApplicationID uuid.UUID      `json:"application_id"`
	Clauses       map[string]any `json:"clauses,omitempty"`
	Notes         string         `json:"notes"`
	CreatedAt     time.Time      `json:"created_at"`
}

type CareerBatchResult struct {
	Evaluated int                    `json:"evaluated"`
	Skipped   int                    `json:"skipped"`
	Results   []CareerEvaluateResult `json:"results"`
}
