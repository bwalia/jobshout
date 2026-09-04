//! Shared domain types for the JobShout.com marketplace.
//!
//! Handlers and repositories share these models. Business rules live in
//! domain services (e.g. `jobshout-jobs`), not in HTTP handlers.

#![forbid(unsafe_code)]

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use uuid::Uuid;

pub type OrganisationId = Uuid;
pub type JobId = Uuid;
pub type UserId = Uuid;
pub type CandidateProfileId = Uuid;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EmploymentType {
    Permanent,
    Contract,
    Freelance,
    Temporary,
    PartTime,
    Internship,
    Apprenticeship,
}

impl EmploymentType {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Permanent => "permanent",
            Self::Contract => "contract",
            Self::Freelance => "freelance",
            Self::Temporary => "temporary",
            Self::PartTime => "part_time",
            Self::Internship => "internship",
            Self::Apprenticeship => "apprenticeship",
        }
    }

    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "permanent" => Some(Self::Permanent),
            "contract" => Some(Self::Contract),
            "freelance" => Some(Self::Freelance),
            "temporary" => Some(Self::Temporary),
            "part_time" => Some(Self::PartTime),
            "internship" => Some(Self::Internship),
            "apprenticeship" => Some(Self::Apprenticeship),
            _ => None,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum JobStatus {
    Draft,
    Published,
    Closed,
    Archived,
}

impl JobStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Draft => "draft",
            Self::Published => "published",
            Self::Closed => "closed",
            Self::Archived => "archived",
        }
    }

    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "draft" => Some(Self::Draft),
            "published" => Some(Self::Published),
            "closed" => Some(Self::Closed),
            "archived" => Some(Self::Archived),
            _ => None,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Location {
    pub country: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub region: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub city: Option<String>,
    #[serde(default)]
    pub remote: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Compensation {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub currency: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub min_amount: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_amount: Option<f64>,
    /// annual | monthly | weekly | daily | hourly | contract
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub period: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Organisation {
    pub id: OrganisationId,
    pub name: String,
    pub slug: String,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Job {
    pub id: JobId,
    pub organisation_id: OrganisationId,
    pub title: String,
    pub summary: String,
    pub description: String,
    pub employment_type: EmploymentType,
    pub location: Location,
    pub compensation: Compensation,
    pub requirements: Vec<String>,
    pub status: JobStatus,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub published_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CreateJobRequest {
    pub title: String,
    #[serde(default)]
    pub summary: String,
    pub description: String,
    pub employment_type: EmploymentType,
    pub location: Location,
    #[serde(default)]
    pub compensation: Compensation,
    #[serde(default)]
    pub requirements: Vec<String>,
    /// When true, create as published; otherwise draft.
    #[serde(default)]
    pub publish: bool,
}

/// Candidate profile shaped for humans and for Career / matching agents.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CandidateProfile {
    pub id: CandidateProfileId,
    pub email: String,
    pub display_name: String,
    pub headline: String,
    pub summary: String,
    pub skills: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub years_experience: Option<i32>,
    pub preferred_roles: Vec<String>,
    pub preferred_locations: Vec<Location>,
    pub preferred_employment_types: Vec<EmploymentType>,
    pub open_to_remote: bool,
    pub salary_expectation: Compensation,
    pub cv_text: String,
    /// Free-text hints the matching agent should respect.
    pub matching_notes: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct UpsertCandidateProfileRequest {
    pub email: String,
    pub display_name: String,
    #[serde(default)]
    pub headline: String,
    #[serde(default)]
    pub summary: String,
    #[serde(default)]
    pub skills: Vec<String>,
    #[serde(default)]
    pub years_experience: Option<i32>,
    #[serde(default)]
    pub preferred_roles: Vec<String>,
    #[serde(default)]
    pub preferred_locations: Vec<Location>,
    #[serde(default)]
    pub preferred_employment_types: Vec<EmploymentType>,
    #[serde(default = "default_true")]
    pub open_to_remote: bool,
    #[serde(default)]
    pub salary_expectation: Compensation,
    #[serde(default)]
    pub cv_text: String,
    #[serde(default)]
    pub matching_notes: String,
}

fn default_true() -> bool {
    true
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JobMatch {
    pub job: Job,
    /// 0–100 explainable score for agents and UI.
    pub score: u8,
    pub reasons: Vec<String>,
}

#[derive(Debug, Error)]
pub enum DomainError {
    #[error("not found")]
    NotFound,
    #[error("validation: {0}")]
    Validation(String),
    #[error("conflict: {0}")]
    Conflict(String),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

/// Stable API error body (prompt §47).
#[derive(Debug, Serialize)]
pub struct ApiErrorBody {
    pub error: ApiErrorDetail,
}

#[derive(Debug, Serialize)]
pub struct ApiErrorDetail {
    pub code: String,
    pub message: String,
    pub request_id: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn employment_type_roundtrip() {
        for v in [
            EmploymentType::Permanent,
            EmploymentType::Contract,
            EmploymentType::PartTime,
            EmploymentType::Apprenticeship,
        ] {
            assert_eq!(EmploymentType::parse(v.as_str()), Some(v));
        }
        assert_eq!(EmploymentType::parse("nope"), None);
    }

    #[test]
    fn job_status_roundtrip() {
        for v in [
            JobStatus::Draft,
            JobStatus::Published,
            JobStatus::Closed,
            JobStatus::Archived,
        ] {
            assert_eq!(JobStatus::parse(v.as_str()), Some(v));
        }
    }
}
