//! Repository layer for freelancer marketplace

use crate::{BidId, ProjectId};
use chrono::Utc;
use jobshout_domain::{
    Attachment, Availability, Bid, BidStatus, Certification, CreateBidRequest,
    CreateProjectRequest, DomainError, FreelancerProfile, Milestone, PaymentMethod, Project,
    ProjectCategory, ProjectType, UpsertFreelancerProfileRequest,
};
use jobshout_observability::error_ctx;
use rust_decimal::Decimal;
use sqlx::{postgres::PgRow, Row};
use std::str::FromStr;
use uuid::Uuid;

#[derive(Clone)]
pub struct ProjectRepository {
    pool: sqlx::PgPool,
}

#[derive(Clone)]
pub struct BidRepository {
    pool: sqlx::PgPool,
}

#[derive(Clone)]
pub struct FreelancerProfileRepository {
    pool: sqlx::PgPool,
}

impl ProjectRepository {
    pub fn new(pool: sqlx::PgPool) -> Self {
        Self { pool }
    }

    /// List projects with filtering
    pub async fn list(
        &self,
        limit: i64,
        offset: i64,
        project_type: Option<ProjectType>,
        category_ids: Option<Vec<Uuid>>,
    ) -> Result<Vec<Project>, DomainError> {
        let mut query = sqlx::QueryBuilder::new(
            r#"
            SELECT
                p.id, p.job_id, p.project_type, p.budget_min, p.budget_max,
                p.currency, p.duration_estimate, p.skills_required, p.deliverables,
                p.milestones, p.bidding_end_at, p.max_bids, p.escrow_required,
                p.created_at, p.updated_at
            FROM projects p
            WHERE 1=1
            "#,
        );

        if let Some(pt) = project_type {
            query.push(" AND p.project_type = ").push_bind(pt.as_str());
        }

        if let Some(categories) = category_ids {
            if !categories.is_empty() {
                query.push(" AND EXISTS (SELECT 1 FROM project_categories_mapping pcm WHERE pcm.project_id = p.id AND pcm.category_id = ANY(").push_bind(categories).push("))");
            }
        }

        query
            .push(" ORDER BY p.created_at DESC LIMIT ")
            .push_bind(limit)
            .push(" OFFSET ")
            .push_bind(offset);

        let rows = query
            .build_query_as::<ProjectRow>()
            .fetch_all(&self.pool)
            .await
            .map_err(|e| {
                error_ctx!("failed to list projects", error = e.to_string());
                DomainError::Other(e.into())
            })?;

        Ok(rows.into_iter().map(|row| row.into_project()).collect())
    }

    /// Get a specific project
    pub async fn get(&self, id: ProjectId) -> Result<Project, DomainError> {
        let row = sqlx::query_as::<_, ProjectRow>(
            r#"
            SELECT
                p.id, p.job_id, p.project_type, p.budget_min, p.budget_max,
                p.currency, p.duration_estimate, p.skills_required, p.deliverables,
                p.milestones, p.bidding_end_at, p.max_bids, p.escrow_required,
                p.created_at, p.updated_at
            FROM projects p
            WHERE p.id = $1
            "#,
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to get project", project_id = id.to_string(), error = e.to_string());
            DomainError::Other(e.into())
        })?;

        row.map(|r| r.into_project())
            .ok_or(DomainError::NotFound)
    }

    /// Create a new project
    pub async fn create(
        &self,
        req: &CreateProjectRequest,
        client_id: Uuid,
    ) -> Result<Project, DomainError> {
        let now = Utc::now();

        let row = sqlx::query_as::<_, ProjectRow>(
            r#"
            INSERT INTO projects (
                job_id, project_type, budget_min, budget_max, currency,
                duration_estimate, skills_required, deliverables, milestones,
                bidding_end_at, max_bids, escrow_required, created_at, updated_at
            )
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
            RETURNING
                id, job_id, project_type, budget_min, budget_max,
                currency, duration_estimate, skills_required, deliverables,
                milestones, bidding_end_at, max_bids, escrow_required,
                created_at, updated_at
            "#,
        )
        .bind(req.job_id)
        .bind(req.project_type.as_str())
        .bind(req.budget_min)
        .bind(req.budget_max)
        .bind(&req.currency)
        .bind(req.duration_estimate.as_deref())
        .bind(&req.skills_required)
        .bind(&req.deliverables)
        .bind(req.milestones.as_ref().map(|m| serde_json::to_value(m).unwrap()))
        .bind(req.bidding_end_at)
        .bind(req.max_bids)
        .bind(req.escrow_required)
        .bind(now)
        .bind(now)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to create project", error = e.to_string());
            DomainError::Other(e.into())
        })?;

        Ok(row.into_project())
    }

    /// Get job details for a project
    pub async fn get_job(&self, job_id: Uuid) -> Result<jobshout_domain::Job, DomainError> {
        // This would need to be implemented with the existing job repository
        // For now, we'll return a placeholder
        Err(DomainError::NotFound)
    }

    /// List project categories
    pub async fn list_categories(&self) -> Result<Vec<ProjectCategory>, DomainError> {
        let rows = sqlx::query_as::<_, CategoryRow>(
            r#"
            SELECT id, name, slug, description, icon, created_at
            FROM project_categories
            ORDER BY name
            "#,
        )
        .fetch_all(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to list categories", error = e.to_string());
            DomainError::Other(e.into())
        })?;

        Ok(rows.into_iter().map(|r| r.into_category()).collect())
    }
}

impl BidRepository {
    pub fn new(pool: sqlx::PgPool) -> Self {
        Self { pool }
    }

    /// List bids for a project
    pub async fn list(
        &self,
        project_id: ProjectId,
        limit: i64,
        offset: i64,
        status: Option<BidStatus>,
    ) -> Result<Vec<Bid>, DomainError> {
        let mut query = sqlx::QueryBuilder::new(
            r#"
            SELECT
                b.id, b.project_id, b.freelancer_id, b.bid_amount, b.hourly_rate,
                b.estimated_hours, b.proposal, b.delivery_timeline, b.attachments,
                b.status, b.is_shortlisted, b.client_rating, b.client_feedback,
                b.freelancer_rating, b.freelancer_feedback, b.created_at, b.updated_at
            FROM bids b
            WHERE b.project_id = $1
            "#,
        );

        query.push_bind(project_id);

        if let Some(s) = status {
            query.push(" AND b.status = ").push_bind(s.as_str());
        }

        query
            .push(" ORDER BY b.created_at ASC LIMIT ")
            .push_bind(limit)
            .push(" OFFSET ")
            .push_bind(offset);

        let rows = query
            .build_query_as::<BidRow>()
            .fetch_all(&self.pool)
            .await
            .map_err(|e| {
                error_ctx!("failed to list bids", project_id = project_id.to_string(), error = e.to_string());
                DomainError::Other(e.into())
            })?;

        Ok(rows.into_iter().map(|row| row.into_bid()).collect())
    }

    /// Get a specific bid
    pub async fn get(&self, id: BidId) -> Result<Bid, DomainError> {
        let row = sqlx::query_as::<_, BidRow>(
            r#"
            SELECT
                b.id, b.project_id, b.freelancer_id, b.bid_amount, b.hourly_rate,
                b.estimated_hours, b.proposal, b.delivery_timeline, b.attachments,
                b.status, b.is_shortlisted, b.client_rating, b.client_feedback,
                b.freelancer_rating, b.freelancer_feedback, b.created_at, b.updated_at
            FROM bids b
            WHERE b.id = $1
            "#,
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to get bid", bid_id = id.to_string(), error = e.to_string());
            DomainError::Other(e.into())
        })?;

        row.map(|r| r.into_bid())
            .ok_or(DomainError::NotFound)
    }

    /// Check if freelancer has already bid on a project
    pub async fn has_bid(&self, project_id: ProjectId, freelancer_id: Uuid) -> Result<bool, DomainError> {
        let count: i64 = sqlx::query_scalar(
            r#"
            SELECT COUNT(*) FROM bids
            WHERE project_id = $1 AND freelancer_id = $2
            "#,
        )
        .bind(project_id)
        .bind(freelancer_id)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to check bid existence", error = e.to_string());
            DomainError::Other(e.into())
        })?;

        Ok(count > 0)
    }

    /// Create a new bid
    pub async fn create(
        &self,
        project_id: ProjectId,
        freelancer_id: Uuid,
        req: &CreateBidRequest,
    ) -> Result<Bid, DomainError> {
        let now = Utc::now();

        let row = sqlx::query_as::<_, BidRow>(
            r#"
            INSERT INTO bids (
                project_id, freelancer_id, bid_amount, hourly_rate, estimated_hours,
                proposal, delivery_timeline, attachments, status, is_shortlisted,
                created_at, updated_at
            )
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
            RETURNING
                id, project_id, freelancer_id, bid_amount, hourly_rate,
                estimated_hours, proposal, delivery_timeline, attachments,
                status, is_shortlisted, client_rating, client_feedback,
                freelancer_rating, freelancer_feedback, created_at, updated_at
            "#,
        )
        .bind(project_id)
        .bind(freelancer_id)
        .bind(req.bid_amount)
        .bind(req.hourly_rate)
        .bind(req.estimated_hours)
        .bind(&req.proposal)
        .bind(req.delivery_timeline.as_deref())
        .bind(req.attachments.as_ref().map(|a| serde_json::to_value(a).unwrap()))
        .bind(BidStatus::Submitted.as_str())
        .bind(false)
        .bind(now)
        .bind(now)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to create bid", error = e.to_string());
            DomainError::Other(e.into())
        })?;

        Ok(row.into_bid())
    }

    /// List bids by freelancer
    pub async fn list_by_freelancer(
        &self,
        freelancer_id: Uuid,
        limit: i64,
        offset: i64,
        status: Option<BidStatus>,
    ) -> Result<Vec<Bid>, DomainError> {
        let mut query = sqlx::QueryBuilder::new(
            r#"
            SELECT
                b.id, b.project_id, b.freelancer_id, b.bid_amount, b.hourly_rate,
                b.estimated_hours, b.proposal, b.delivery_timeline, b.attachments,
                b.status, b.is_shortlisted, b.client_rating, b.client_feedback,
                b.freelancer_rating, b.freelancer_feedback, b.created_at, b.updated_at
            FROM bids b
            WHERE b.freelancer_id = $1
            "#,
        );

        query.push_bind(freelancer_id);

        if let Some(s) = status {
            query.push(" AND b.status = ").push_bind(s.as_str());
        }

        query
            .push(" ORDER BY b.created_at DESC LIMIT ")
            .push_bind(limit)
            .push(" OFFSET ")
            .push_bind(offset);

        let rows = query
            .build_query_as::<BidRow>()
            .fetch_all(&self.pool)
            .await
            .map_err(|e| {
                error_ctx!("failed to list freelancer bids", freelancer_id = freelancer_id.to_string(), error = e.to_string());
                DomainError::Other(e.into())
            })?;

        Ok(rows.into_iter().map(|row| row.into_bid()).collect())
    }

    /// Update bid status
    pub async fn update_status(&self, id: BidId, status: BidStatus) -> Result<Bid, DomainError> {
        let now = Utc::now();

        let row = sqlx::query_as::<_, BidRow>(
            r#"
            UPDATE bids
            SET status = $1, updated_at = $2, is_shortlisted = $3
            WHERE id = $4
            RETURNING
                id, project_id, freelancer_id, bid_amount, hourly_rate,
                estimated_hours, proposal, delivery_timeline, attachments,
                status, is_shortlisted, client_rating, client_feedback,
                freelancer_rating, freelancer_feedback, created_at, updated_at
            "#,
        )
        .bind(status.as_str())
        .bind(now)
        .bind(status == BidStatus::Shortlisted)
        .bind(id)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to update bid status", bid_id = id.to_string(), error = e.to_string());
            DomainError::Other(e.into())
        })?;

        Ok(row.into_bid())
    }
}

impl FreelancerProfileRepository {
    pub fn new(pool: sqlx::PgPool) -> Self {
        Self { pool }
    }

    /// Get freelancer profile by user ID
    pub async fn get_by_user_id(&self, user_id: Uuid) -> Result<Option<FreelancerProfile>, DomainError> {
        let row = sqlx::query_as::<_, FreelancerProfileRow>(
            r#"
            SELECT
                fp.id, fp.candidate_profile_id, fp.user_id, fp.hourly_rate_min,
                fp.hourly_rate_max, fp.availability, fp.portfolio_urls,
                fp.years_experience, fp.success_score, fp.completed_projects,
                fp.client_satisfaction, fp.on_time_delivery, fp.skills_certifications,
                fp.verified_identity, fp.payment_methods, fp.created_at, fp.updated_at
            FROM freelancer_profiles fp
            WHERE fp.user_id = $1
            "#,
        )
        .bind(user_id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to get freelancer profile", user_id = user_id.to_string(), error = e.to_string());
            DomainError::Other(e.into())
        })?;

        Ok(row.map(|r| r.into_profile()))
    }

    /// Create or update freelancer profile
    pub async fn upsert(
        &self,
        user_id: Uuid,
        req: &UpsertFreelancerProfileRequest,
    ) -> Result<FreelancerProfile, DomainError> {
        let now = Utc::now();

        let row = sqlx::query_as::<_, FreelancerProfileRow>(
            r#"
            INSERT INTO freelancer_profiles (
                user_id, hourly_rate_min, hourly_rate_max, availability,
                portfolio_urls, years_experience, skills_certifications,
                payment_methods, created_at, updated_at
            )
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
            ON CONFLICT (user_id) DO UPDATE SET
                hourly_rate_min = EXCLUDED.hourly_rate_min,
                hourly_rate_max = EXCLUDED.hourly_rate_max,
                availability = EXCLUDED.availability,
                portfolio_urls = EXCLUDED.portfolio_urls,
                years_experience = EXCLUDED.years_experience,
                skills_certifications = EXCLUDED.skills_certifications,
                payment_methods = EXCLUDED.payment_methods,
                updated_at = EXCLUDED.updated_at
            RETURNING
                id, candidate_profile_id, user_id, hourly_rate_min,
                hourly_rate_max, availability, portfolio_urls,
                years_experience, success_score, completed_projects,
                client_satisfaction, on_time_delivery, skills_certifications,
                verified_identity, payment_methods, created_at, updated_at
            "#,
        )
        .bind(user_id)
        .bind(req.hourly_rate_min)
        .bind(req.hourly_rate_max)
        .bind(req.availability.map(|a| a.as_str()))
        .bind(&req.portfolio_urls)
        .bind(req.years_experience)
        .bind(serde_json::to_value(&req.skills_certifications).unwrap())
        .bind(serde_json::to_value(&req.payment_methods).unwrap())
        .bind(now)
        .bind(now)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            error_ctx!("failed to upsert freelancer profile", error = e.to_string());
            DomainError::Other(e.into())
        })?;

        Ok(row.into_profile())
    }
}

// Database row mappings

struct ProjectRow {
    id: Uuid,
    job_id: Uuid,
    project_type: String,
    budget_min: Option<Decimal>,
    budget_max: Option<Decimal>,
    currency: String,
    duration_estimate: Option<String>,
    skills_required: Vec<String>,
    deliverables: Vec<String>,
    milestones: Option<serde_json::Value>,
    bidding_end_at: Option<chrono::DateTime<Utc>>,
    max_bids: Option<i32>,
    escrow_required: bool,
    created_at: chrono::DateTime<Utc>,
    updated_at: chrono::DateTime<Utc>,
}

impl ProjectRow {
    fn into_project(self) -> Project {
        let milestones = self.milestones.and_then(|m| {
            serde_json::from_value::<Vec<Milestone>>(m).ok()
        });

        Project {
            id: self.id,
            job_id: self.job_id,
            project_type: ProjectType::parse(&self.project_type).unwrap_or(ProjectType::FixedPrice),
            budget_min: self.budget_min,
            budget_max: self.budget_max,
            currency: self.currency,
            duration_estimate: self.duration_estimate,
            skills_required: self.skills_required,
            deliverables: self.deliverables,
            milestones,
            bidding_end_at: self.bidding_end_at,
            max_bids: self.max_bids,
            escrow_required: self.escrow_required,
            created_at: self.created_at,
            updated_at: self.updated_at,
        }
    }
}

impl<'r> sqlx::FromRow<'r, PgRow> for ProjectRow {
    fn from_row(row: &'r PgRow) -> sqlx::Result<Self> {
        Ok(Self {
            id: row.try_get("id")?,
            job_id: row.try_get("job_id")?,
            project_type: row.try_get("project_type")?,
            budget_min: row.try_get("budget_min")?,
            budget_max: row.try_get("budget_max")?,
            currency: row.try_get("currency")?,
            duration_estimate: row.try_get("duration_estimate")?,
            skills_required: row.try_get("skills_required")?,
            deliverables: row.try_get("deliverables")?,
            milestones: row.try_get("milestones")?,
            bidding_end_at: row.try_get("bidding_end_at")?,
            max_bids: row.try_get("max_bids")?,
            escrow_required: row.try_get("escrow_required")?,
            created_at: row.try_get("created_at")?,
            updated_at: row.try_get("updated_at")?,
        })
    }
}

struct BidRow {
    id: Uuid,
    project_id: Uuid,
    freelancer_id: Uuid,
    bid_amount: Option<Decimal>,
    hourly_rate: Option<Decimal>,
    estimated_hours: Option<i32>,
    proposal: String,
    delivery_timeline: Option<String>,
    attachments: Option<serde_json::Value>,
    status: String,
    is_shortlisted: bool,
    client_rating: Option<i32>,
    client_feedback: Option<String>,
    freelancer_rating: Option<i32>,
    freelancer_feedback: Option<String>,
    created_at: chrono::DateTime<Utc>,
    updated_at: chrono::DateTime<Utc>,
}

impl BidRow {
    fn into_bid(self) -> Bid {
        let attachments = self.attachments.and_then(|a| {
            serde_json::from_value::<Vec<Attachment>>(a).ok()
        });

        Bid {
            id: self.id,
            project_id: self.project_id,
            freelancer_id: self.freelancer_id,
            bid_amount: self.bid_amount,
            hourly_rate: self.hourly_rate,
            estimated_hours: self.estimated_hours,
            proposal: self.proposal,
            delivery_timeline: self.delivery_timeline,
            attachments,
            status: BidStatus::parse(&self.status).unwrap_or(BidStatus::Submitted),
            is_shortlisted: self.is_shortlisted,
            client_rating: self.client_rating,
            client_feedback: self.client_feedback,
            freelancer_rating: self.freelancer_rating,
            freelancer_feedback: self.freelancer_feedback,
            created_at: self.created_at,
            updated_at: self.updated_at,
        }
    }
}

impl<'r> sqlx::FromRow<'r, PgRow> for BidRow {
    fn from_row(row: &'r PgRow) -> sqlx::Result<Self> {
        Ok(Self {
            id: row.try_get("id")?,
            project_id: row.try_get("project_id")?,
            freelancer_id: row.try_get("freelancer_id")?,
            bid_amount: row.try_get("bid_amount")?,
            hourly_rate: row.try_get("hourly_rate")?,
            estimated_hours: row.try_get("estimated_hours")?,
            proposal: row.try_get("proposal")?,
            delivery_timeline: row.try_get("delivery_timeline")?,
            attachments: row.try_get("attachments")?,
            status: row.try_get("status")?,
            is_shortlisted: row.try_get("is_shortlisted")?,
            client_rating: row.try_get("client_rating")?,
            client_feedback: row.try_get("client_feedback")?,
            freelancer_rating: row.try_get("freelancer_rating")?,
            freelancer_feedback: row.try_get("freelancer_feedback")?,
            created_at: row.try_get("created_at")?,
            updated_at: row.try_get("updated_at")?,
        })
    }
}

struct FreelancerProfileRow {
    id: Uuid,
    candidate_profile_id: Option<Uuid>,
    user_id: Uuid,
    hourly_rate_min: Option<Decimal>,
    hourly_rate_max: Option<Decimal>,
    availability: Option<String>,
    portfolio_urls: Vec<String>,
    years_experience: Option<i32>,
    success_score: f32,
    completed_projects: i32,
    client_satisfaction: f32,
    on_time_delivery: f32,
    skills_certifications: serde_json::Value,
    verified_identity: bool,
    payment_methods: serde_json::Value,
    created_at: chrono::DateTime<Utc>,
    updated_at: chrono::DateTime<Utc>,
}

impl FreelancerProfileRow {
    fn into_profile(self) -> FreelancerProfile {
        let skills_certifications: Vec<Certification> = serde_json::from_value(self.skills_certifications)
            .unwrap_or_default();
        let payment_methods: Vec<PaymentMethod> = serde_json::from_value(self.payment_methods)
            .unwrap_or_default();

        FreelancerProfile {
            id: self.id,
            candidate_profile_id: self.candidate_profile_id,
            user_id: self.user_id,
            hourly_rate_min: self.hourly_rate_min,
            hourly_rate_max: self.hourly_rate_max,
            availability: self.availability.and_then(|a| Availability::parse(&a)),
            portfolio_urls: self.portfolio_urls,
            years_experience: self.years_experience,
            success_score: self.success_score,
            completed_projects: self.completed_projects,
            client_satisfaction: self.client_satisfaction,
            on_time_delivery: self.on_time_delivery,
            skills_certifications,
            verified_identity: self.verified_identity,
            payment_methods,
            created_at: self.created_at,
            updated_at: self.updated_at,
        }
    }
}

impl<'r> sqlx::FromRow<'r, PgRow> for FreelancerProfileRow {
    fn from_row(row: &'r PgRow) -> sqlx::Result<Self> {
        Ok(Self {
            id: row.try_get("id")?,
            candidate_profile_id: row.try_get("candidate_profile_id")?,
            user_id: row.try_get("user_id")?,
            hourly_rate_min: row.try_get("hourly_rate_min")?,
            hourly_rate_max: row.try_get("hourly_rate_max")?,
            availability: row.try_get("availability")?,
            portfolio_urls: row.try_get("portfolio_urls")?,
            years_experience: row.try_get("years_experience")?,
            success_score: row.try_get("success_score")?,
            completed_projects: row.try_get("completed_projects")?,
            client_satisfaction: row.try_get("client_satisfaction")?,
            on_time_delivery: row.try_get("on_time_delivery")?,
            skills_certifications: row.try_get("skills_certifications")?,
            verified_identity: row.try_get("verified_identity")?,
            payment_methods: row.try_get("payment_methods")?,
            created_at: row.try_get("created_at")?,
            updated_at: row.try_get("updated_at")?,
        })
    }
}

struct CategoryRow {
    id: Uuid,
    name: String,
    slug: String,
    description: Option<String>,
    icon: Option<String>,
    created_at: chrono::DateTime<Utc>,
}

impl CategoryRow {
    fn into_category(self) -> ProjectCategory {
        ProjectCategory {
            id: self.id,
            name: self.name,
            slug: self.slug,
            description: self.description,
            icon: self.icon,
            created_at: self.created_at,
        }
    }
}

impl<'r> sqlx::FromRow<'r, PgRow> for CategoryRow {
    fn from_row(row: &'r PgRow) -> sqlx::Result<Self> {
        Ok(Self {
            id: row.try_get("id")?,
            name: row.try_get("name")?,
            slug: row.try_get("slug")?,
            description: row.try_get("description")?,
            icon: row.try_get("icon")?,
            created_at: row.try_get("created_at")?,
        })
    }
}