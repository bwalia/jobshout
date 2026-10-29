//! Domain models for Upwork-style freelancer marketplace

use chrono::{DateTime, Utc};
use rust_decimal::Decimal;
use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// Types of projects in the freelancer marketplace
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ProjectType {
    FixedPrice,
    Hourly,
    Retainer,
    Milestone,
}

impl ProjectType {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::FixedPrice => "fixed_price",
            Self::Hourly => "hourly",
            Self::Retainer => "retainer",
            Self::Milestone => "milestone",
        }
    }

    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "fixed_price" => Some(Self::FixedPrice),
            "hourly" => Some(Self::Hourly),
            "retainer" => Some(Self::Retainer),
            "milestone" => Some(Self::Milestone),
            _ => None,
        }
    }
}

/// Status of a bid on a project
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum BidStatus {
    Submitted,
    Shortlisted,
    Rejected,
    Accepted,
    Withdrawn,
}

impl BidStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Submitted => "submitted",
            Self::Shortlisted => "shortlisted",
            Self::Rejected => "rejected",
            Self::Accepted => "accepted",
            Self::Withdrawn => "withdrawn",
        }
    }

    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "submitted" => Some(Self::Submitted),
            "shortlisted" => Some(Self::Shortlisted),
            "rejected" => Some(Self::Rejected),
            "accepted" => Some(Self::Accepted),
            "withdrawn" => Some(Self::Withdrawn),
            _ => None,
        }
    }
}

/// Freelancer availability status
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Availability {
    FullTime,
    PartTime,
    NotAvailable,
}

impl Availability {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::FullTime => "full_time",
            Self::PartTime => "part_time",
            Self::NotAvailable => "not_available",
        }
    }

    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "full_time" => Some(Self::FullTime),
            "part_time" => Some(Self::PartTime),
            "not_available" => Some(Self::NotAvailable),
            _ => None,
        }
    }
}

/// Milestone structure for milestone-based projects
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Milestone {
    pub name: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub amount: Decimal,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub due_date: Option<DateTime<Utc>>,
}

/// Project model for freelancer marketplace
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Project {
    pub id: Uuid,
    pub job_id: Uuid,
    pub project_type: ProjectType,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub budget_min: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub budget_max: Option<Decimal>,
    pub currency: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub duration_estimate: Option<String>,
    pub skills_required: Vec<String>,
    pub deliverables: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub milestones: Option<Vec<Milestone>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bidding_end_at: Option<DateTime<Utc>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_bids: Option<i32>,
    pub escrow_required: bool,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Request to create a new project
#[derive(Debug, Clone, Deserialize)]
pub struct CreateProjectRequest {
    pub title: String,
    pub description: String,
    #[serde(default)]
    pub summary: String,
    pub project_type: ProjectType,
    #[serde(default)]
    pub budget_min: Option<Decimal>,
    #[serde(default)]
    pub budget_max: Option<Decimal>,
    #[serde(default = "default_currency")]
    pub currency: String,
    #[serde(default)]
    pub duration_estimate: Option<String>,
    #[serde(default)]
    pub skills_required: Vec<String>,
    #[serde(default)]
    pub deliverables: Vec<String>,
    #[serde(default)]
    pub milestones: Option<Vec<Milestone>>,
    #[serde(default)]
    pub bidding_end_at: Option<DateTime<Utc>>,
    #[serde(default)]
    pub max_bids: Option<i32>,
    #[serde(default)]
    pub escrow_required: bool,
    #[serde(default)]
    pub requirements: Vec<String>,
}

fn default_currency() -> String {
    "USD".to_string()
}

/// Bid on a project
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Bid {
    pub id: Uuid,
    pub project_id: Uuid,
    pub freelancer_id: Uuid,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bid_amount: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hourly_rate: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub estimated_hours: Option<i32>,
    pub proposal: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub delivery_timeline: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub attachments: Option<Vec<Attachment>>,
    pub status: BidStatus,
    pub is_shortlisted: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub client_rating: Option<i32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub client_feedback: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub freelancer_rating: Option<i32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub freelancer_feedback: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Request to submit a bid
#[derive(Debug, Clone, Deserialize)]
pub struct CreateBidRequest {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bid_amount: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hourly_rate: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub estimated_hours: Option<i32>,
    pub proposal: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub delivery_timeline: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub attachments: Option<Vec<Attachment>>,
}

/// Attachment for bids
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Attachment {
    pub name: String,
    pub url: String,
    #[serde(rename = "type")]
    pub file_type: String,
    pub size: Option<i64>,
}

/// Freelancer profile extending candidate profile
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FreelancerProfile {
    pub id: Uuid,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub candidate_profile_id: Option<Uuid>,
    pub user_id: Uuid,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hourly_rate_min: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hourly_rate_max: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub availability: Option<Availability>,
    pub portfolio_urls: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub years_experience: Option<i32>,
    pub success_score: f32,
    pub completed_projects: i32,
    pub client_satisfaction: f32,
    pub on_time_delivery: f32,
    pub skills_certifications: Vec<Certification>,
    pub verified_identity: bool,
    pub payment_methods: Vec<PaymentMethod>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Certification or skill proof
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Certification {
    pub name: String,
    pub issuer: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub issue_date: Option<DateTime<Utc>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub expiry_date: Option<DateTime<Utc>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub credential_url: Option<String>,
}

/// Payment method for freelancer
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaymentMethod {
    pub provider: String,
    pub account_id: String,
    pub verified: bool,
}

/// Request to create/update freelancer profile
#[derive(Debug, Clone, Deserialize)]
pub struct UpsertFreelancerProfileRequest {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hourly_rate_min: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub hourly_rate_max: Option<Decimal>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub availability: Option<Availability>,
    #[serde(default)]
    pub portfolio_urls: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub years_experience: Option<i32>,
    #[serde(default)]
    pub skills_certifications: Vec<Certification>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub payment_methods: Vec<PaymentMethod>,
}

/// Project category for filtering
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectCategory {
    pub id: Uuid,
    pub name: String,
    pub slug: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub icon: Option<String>,
    pub created_at: DateTime<Utc>,
}

/// Project with its job details
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectWithJob {
    #[serde(flatten)]
    pub project: Project,
    pub job: super::Job,
    pub categories: Vec<ProjectCategory>,
}

/// Bid with project details
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BidWithProject {
    #[serde(flatten)]
    pub bid: Bid,
    pub project: Project,
    pub job: super::Job,
}