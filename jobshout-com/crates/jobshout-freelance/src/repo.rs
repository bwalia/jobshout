//! Repository layer for freelancer marketplace.

use crate::{BidId, ProjectId};
use chrono::Utc;
use jobshout_domain::{
    Attachment, Availability, Bid, BidStatus, Certification, CreateBidRequest,
    CreateProjectRequest, DomainError, FreelancerProfile, Milestone, PaymentMethod, Project,
    ProjectCategory, ProjectType, UpsertFreelancerProfileRequest,
};
use sqlx::{postgres::PgRow, Row};
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

    pub async fn list(
        &self,
        limit: i64,
        offset: i64,
        project_type: Option<ProjectType>,
        category_ids: Option<Vec<Uuid>>,
    ) -> Result<Vec<Project>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT
                p.id, p.job_id, p.project_type, p.budget_min, p.budget_max,
                p.currency, p.duration_estimate, p.skills_required, p.deliverables,
                p.milestones, p.bidding_end_at, p.max_bids, p.escrow_required,
                p.created_at, p.updated_at
            FROM projects p
            WHERE ($1::text IS NULL OR p.project_type = $1)
              AND (
                $2::uuid[] IS NULL
                OR EXISTS (
                    SELECT 1 FROM project_categories_mapping pcm
                    WHERE pcm.project_id = p.id AND pcm.category_id = ANY($2)
                )
              )
            ORDER BY p.created_at DESC
            LIMIT $3 OFFSET $4
            "#,
        )
        .bind(project_type.map(|pt| pt.as_str()))
        .bind(category_ids)
        .bind(limit)
        .bind(offset)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to list projects");
            DomainError::Other(e.into())
        })?;

        rows.into_iter().map(project_from_row).collect()
    }

    pub async fn get(&self, id: ProjectId) -> Result<Project, DomainError> {
        let row = sqlx::query(
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
            tracing::error!(project_id = %id, error = %e, "failed to get project");
            DomainError::Other(e.into())
        })?;

        match row {
            Some(r) => project_from_row(r),
            None => Err(DomainError::NotFound),
        }
    }

    pub async fn create(&self, req: &CreateProjectRequest) -> Result<Project, DomainError> {
        let now = Utc::now();
        let milestones = req
            .milestones
            .as_ref()
            .map(serde_json::to_value)
            .transpose()
            .map_err(|e| DomainError::Other(e.into()))?;

        let row = sqlx::query(
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
        .bind(Option::<Uuid>::None)
        .bind(req.project_type.as_str())
        .bind(req.budget_min)
        .bind(req.budget_max)
        .bind(&req.currency)
        .bind(req.duration_estimate.as_deref())
        .bind(&req.skills_required)
        .bind(&req.deliverables)
        .bind(milestones)
        .bind(req.bidding_end_at)
        .bind(req.max_bids)
        .bind(req.escrow_required)
        .bind(now)
        .bind(now)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to create project");
            DomainError::Other(e.into())
        })?;

        project_from_row(row)
    }

    pub async fn list_categories(&self) -> Result<Vec<ProjectCategory>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT id, name, slug, description, icon, created_at
            FROM project_categories
            ORDER BY name
            "#,
        )
        .fetch_all(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to list categories");
            DomainError::Other(e.into())
        })?;

        rows.into_iter()
            .map(|row| {
                Ok(ProjectCategory {
                    id: row.try_get("id")?,
                    name: row.try_get("name")?,
                    slug: row.try_get("slug")?,
                    description: row.try_get("description")?,
                    icon: row.try_get("icon")?,
                    created_at: row.try_get("created_at")?,
                })
            })
            .collect::<Result<Vec<_>, sqlx::Error>>()
            .map_err(|e| DomainError::Other(e.into()))
    }
}

impl BidRepository {
    pub fn new(pool: sqlx::PgPool) -> Self {
        Self { pool }
    }

    pub async fn list(
        &self,
        project_id: ProjectId,
        limit: i64,
        offset: i64,
        status: Option<BidStatus>,
    ) -> Result<Vec<Bid>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT
                b.id, b.project_id, b.freelancer_id, b.bid_amount, b.hourly_rate,
                b.estimated_hours, b.proposal, b.delivery_timeline, b.attachments,
                b.status, b.is_shortlisted, b.client_rating, b.client_feedback,
                b.freelancer_rating, b.freelancer_feedback, b.created_at, b.updated_at
            FROM bids b
            WHERE b.project_id = $1
              AND ($2::text IS NULL OR b.status = $2)
            ORDER BY b.created_at ASC
            LIMIT $3 OFFSET $4
            "#,
        )
        .bind(project_id)
        .bind(status.map(|s| s.as_str()))
        .bind(limit)
        .bind(offset)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(project_id = %project_id, error = %e, "failed to list bids");
            DomainError::Other(e.into())
        })?;

        rows.into_iter().map(bid_from_row).collect()
    }

    pub async fn get(&self, id: BidId) -> Result<Bid, DomainError> {
        let row = sqlx::query(
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
            tracing::error!(bid_id = %id, error = %e, "failed to get bid");
            DomainError::Other(e.into())
        })?;

        match row {
            Some(r) => bid_from_row(r),
            None => Err(DomainError::NotFound),
        }
    }

    pub async fn has_bid(
        &self,
        project_id: ProjectId,
        freelancer_id: Uuid,
    ) -> Result<bool, DomainError> {
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
            tracing::error!(error = %e, "failed to check bid existence");
            DomainError::Other(e.into())
        })?;
        Ok(count > 0)
    }

    pub async fn create(
        &self,
        project_id: ProjectId,
        freelancer_id: Uuid,
        req: &CreateBidRequest,
    ) -> Result<Bid, DomainError> {
        let now = Utc::now();
        let attachments = req
            .attachments
            .as_ref()
            .map(serde_json::to_value)
            .transpose()
            .map_err(|e| DomainError::Other(e.into()))?;

        let row = sqlx::query(
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
        .bind(attachments)
        .bind(BidStatus::Submitted.as_str())
        .bind(false)
        .bind(now)
        .bind(now)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to create bid");
            DomainError::Other(e.into())
        })?;

        bid_from_row(row)
    }

    pub async fn list_by_freelancer(
        &self,
        freelancer_id: Uuid,
        limit: i64,
        offset: i64,
        status: Option<BidStatus>,
    ) -> Result<Vec<Bid>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT
                b.id, b.project_id, b.freelancer_id, b.bid_amount, b.hourly_rate,
                b.estimated_hours, b.proposal, b.delivery_timeline, b.attachments,
                b.status, b.is_shortlisted, b.client_rating, b.client_feedback,
                b.freelancer_rating, b.freelancer_feedback, b.created_at, b.updated_at
            FROM bids b
            WHERE b.freelancer_id = $1
              AND ($2::text IS NULL OR b.status = $2)
            ORDER BY b.created_at DESC
            LIMIT $3 OFFSET $4
            "#,
        )
        .bind(freelancer_id)
        .bind(status.map(|s| s.as_str()))
        .bind(limit)
        .bind(offset)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(
                freelancer_id = %freelancer_id,
                error = %e,
                "failed to list freelancer bids"
            );
            DomainError::Other(e.into())
        })?;

        rows.into_iter().map(bid_from_row).collect()
    }

    pub async fn update_status(&self, id: BidId, status: BidStatus) -> Result<Bid, DomainError> {
        let now = Utc::now();
        let row = sqlx::query(
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
            tracing::error!(bid_id = %id, error = %e, "failed to update bid status");
            DomainError::Other(e.into())
        })?;

        bid_from_row(row)
    }
}

impl FreelancerProfileRepository {
    pub fn new(pool: sqlx::PgPool) -> Self {
        Self { pool }
    }

    pub async fn get_by_candidate_id(
        &self,
        candidate_profile_id: Uuid,
    ) -> Result<Option<FreelancerProfile>, DomainError> {
        let row = sqlx::query(
            r#"
            SELECT
                fp.id, fp.candidate_profile_id, fp.hourly_rate_min,
                fp.hourly_rate_max, fp.availability, fp.portfolio_urls,
                fp.years_experience, fp.success_score, fp.completed_projects,
                fp.client_satisfaction, fp.on_time_delivery, fp.skills_certifications,
                fp.verified_identity, fp.payment_methods, fp.created_at, fp.updated_at
            FROM freelancer_profiles fp
            WHERE fp.candidate_profile_id = $1
            "#,
        )
        .bind(candidate_profile_id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(
                candidate_profile_id = %candidate_profile_id,
                error = %e,
                "failed to get freelancer profile"
            );
            DomainError::Other(e.into())
        })?;

        row.map(profile_from_row).transpose()
    }

    pub async fn upsert(
        &self,
        candidate_profile_id: Uuid,
        req: &UpsertFreelancerProfileRequest,
    ) -> Result<FreelancerProfile, DomainError> {
        let now = Utc::now();
        let skills = serde_json::to_value(&req.skills_certifications)
            .map_err(|e| DomainError::Other(e.into()))?;
        let payments =
            serde_json::to_value(&req.payment_methods).map_err(|e| DomainError::Other(e.into()))?;

        let row = sqlx::query(
            r#"
            INSERT INTO freelancer_profiles (
                candidate_profile_id, hourly_rate_min, hourly_rate_max, availability,
                portfolio_urls, years_experience, skills_certifications,
                payment_methods, created_at, updated_at
            )
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
            ON CONFLICT (candidate_profile_id) DO UPDATE SET
                hourly_rate_min = EXCLUDED.hourly_rate_min,
                hourly_rate_max = EXCLUDED.hourly_rate_max,
                availability = EXCLUDED.availability,
                portfolio_urls = EXCLUDED.portfolio_urls,
                years_experience = EXCLUDED.years_experience,
                skills_certifications = EXCLUDED.skills_certifications,
                payment_methods = EXCLUDED.payment_methods,
                updated_at = EXCLUDED.updated_at
            RETURNING
                id, candidate_profile_id, hourly_rate_min,
                hourly_rate_max, availability, portfolio_urls,
                years_experience, success_score, completed_projects,
                client_satisfaction, on_time_delivery, skills_certifications,
                verified_identity, payment_methods, created_at, updated_at
            "#,
        )
        .bind(candidate_profile_id)
        .bind(req.hourly_rate_min)
        .bind(req.hourly_rate_max)
        .bind(req.availability.map(|a| a.as_str()))
        .bind(&req.portfolio_urls)
        .bind(req.years_experience)
        .bind(skills)
        .bind(payments)
        .bind(now)
        .bind(now)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to upsert freelancer profile");
            DomainError::Other(e.into())
        })?;

        profile_from_row(row)
    }
}

fn project_from_row(row: PgRow) -> Result<Project, DomainError> {
    let project_type: String = row
        .try_get("project_type")
        .map_err(|e| DomainError::Other(e.into()))?;
    let milestones: Option<serde_json::Value> = row
        .try_get("milestones")
        .map_err(|e| DomainError::Other(e.into()))?;
    Ok(Project {
        id: row
            .try_get("id")
            .map_err(|e| DomainError::Other(e.into()))?,
        job_id: row
            .try_get("job_id")
            .map_err(|e| DomainError::Other(e.into()))?,
        project_type: ProjectType::parse(&project_type).unwrap_or(ProjectType::FixedPrice),
        budget_min: row
            .try_get("budget_min")
            .map_err(|e| DomainError::Other(e.into()))?,
        budget_max: row
            .try_get("budget_max")
            .map_err(|e| DomainError::Other(e.into()))?,
        currency: row
            .try_get("currency")
            .map_err(|e| DomainError::Other(e.into()))?,
        duration_estimate: row
            .try_get("duration_estimate")
            .map_err(|e| DomainError::Other(e.into()))?,
        skills_required: row
            .try_get("skills_required")
            .map_err(|e| DomainError::Other(e.into()))?,
        deliverables: row
            .try_get("deliverables")
            .map_err(|e| DomainError::Other(e.into()))?,
        milestones: milestones.and_then(|m| serde_json::from_value::<Vec<Milestone>>(m).ok()),
        bidding_end_at: row
            .try_get("bidding_end_at")
            .map_err(|e| DomainError::Other(e.into()))?,
        max_bids: row
            .try_get("max_bids")
            .map_err(|e| DomainError::Other(e.into()))?,
        escrow_required: row
            .try_get("escrow_required")
            .map_err(|e| DomainError::Other(e.into()))?,
        created_at: row
            .try_get("created_at")
            .map_err(|e| DomainError::Other(e.into()))?,
        updated_at: row
            .try_get("updated_at")
            .map_err(|e| DomainError::Other(e.into()))?,
    })
}

fn bid_from_row(row: PgRow) -> Result<Bid, DomainError> {
    let status: String = row
        .try_get("status")
        .map_err(|e| DomainError::Other(e.into()))?;
    let attachments: Option<serde_json::Value> = row
        .try_get("attachments")
        .map_err(|e| DomainError::Other(e.into()))?;
    Ok(Bid {
        id: row
            .try_get("id")
            .map_err(|e| DomainError::Other(e.into()))?,
        project_id: row
            .try_get("project_id")
            .map_err(|e| DomainError::Other(e.into()))?,
        freelancer_id: row
            .try_get("freelancer_id")
            .map_err(|e| DomainError::Other(e.into()))?,
        bid_amount: row
            .try_get("bid_amount")
            .map_err(|e| DomainError::Other(e.into()))?,
        hourly_rate: row
            .try_get("hourly_rate")
            .map_err(|e| DomainError::Other(e.into()))?,
        estimated_hours: row
            .try_get("estimated_hours")
            .map_err(|e| DomainError::Other(e.into()))?,
        proposal: row
            .try_get("proposal")
            .map_err(|e| DomainError::Other(e.into()))?,
        delivery_timeline: row
            .try_get("delivery_timeline")
            .map_err(|e| DomainError::Other(e.into()))?,
        attachments: attachments.and_then(|a| serde_json::from_value::<Vec<Attachment>>(a).ok()),
        status: BidStatus::parse(&status).unwrap_or(BidStatus::Submitted),
        is_shortlisted: row
            .try_get("is_shortlisted")
            .map_err(|e| DomainError::Other(e.into()))?,
        client_rating: row
            .try_get("client_rating")
            .map_err(|e| DomainError::Other(e.into()))?,
        client_feedback: row
            .try_get("client_feedback")
            .map_err(|e| DomainError::Other(e.into()))?,
        freelancer_rating: row
            .try_get("freelancer_rating")
            .map_err(|e| DomainError::Other(e.into()))?,
        freelancer_feedback: row
            .try_get("freelancer_feedback")
            .map_err(|e| DomainError::Other(e.into()))?,
        created_at: row
            .try_get("created_at")
            .map_err(|e| DomainError::Other(e.into()))?,
        updated_at: row
            .try_get("updated_at")
            .map_err(|e| DomainError::Other(e.into()))?,
    })
}

fn profile_from_row(row: PgRow) -> Result<FreelancerProfile, DomainError> {
    let availability: Option<String> = row
        .try_get("availability")
        .map_err(|e| DomainError::Other(e.into()))?;
    let skills: serde_json::Value = row
        .try_get("skills_certifications")
        .map_err(|e| DomainError::Other(e.into()))?;
    let payments: serde_json::Value = row
        .try_get("payment_methods")
        .map_err(|e| DomainError::Other(e.into()))?;
    Ok(FreelancerProfile {
        id: row
            .try_get("id")
            .map_err(|e| DomainError::Other(e.into()))?,
        candidate_profile_id: row
            .try_get("candidate_profile_id")
            .map_err(|e| DomainError::Other(e.into()))?,
        hourly_rate_min: row
            .try_get("hourly_rate_min")
            .map_err(|e| DomainError::Other(e.into()))?,
        hourly_rate_max: row
            .try_get("hourly_rate_max")
            .map_err(|e| DomainError::Other(e.into()))?,
        availability: availability.and_then(|a| Availability::parse(&a)),
        portfolio_urls: row
            .try_get("portfolio_urls")
            .map_err(|e| DomainError::Other(e.into()))?,
        years_experience: row
            .try_get("years_experience")
            .map_err(|e| DomainError::Other(e.into()))?,
        success_score: row
            .try_get("success_score")
            .map_err(|e| DomainError::Other(e.into()))?,
        completed_projects: row
            .try_get("completed_projects")
            .map_err(|e| DomainError::Other(e.into()))?,
        client_satisfaction: row
            .try_get("client_satisfaction")
            .map_err(|e| DomainError::Other(e.into()))?,
        on_time_delivery: row
            .try_get("on_time_delivery")
            .map_err(|e| DomainError::Other(e.into()))?,
        skills_certifications: serde_json::from_value::<Vec<Certification>>(skills)
            .unwrap_or_default(),
        verified_identity: row
            .try_get("verified_identity")
            .map_err(|e| DomainError::Other(e.into()))?,
        payment_methods: serde_json::from_value::<Vec<PaymentMethod>>(payments).unwrap_or_default(),
        created_at: row
            .try_get("created_at")
            .map_err(|e| DomainError::Other(e.into()))?,
        updated_at: row
            .try_get("updated_at")
            .map_err(|e| DomainError::Other(e.into()))?,
    })
}
