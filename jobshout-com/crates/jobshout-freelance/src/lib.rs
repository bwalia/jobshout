//! Freelancer marketplace service for Upwork-style projects and bidding.

#![forbid(unsafe_code)]

mod repo;

use jobshout_domain::{
    Bid, BidStatus, CreateBidRequest, CreateProjectRequest, DomainError, FreelancerProfile,
    Project, ProjectCategory, ProjectType, UpsertFreelancerProfileRequest, UserId,
};
use sqlx::PgPool;
use uuid::Uuid;

pub use repo::{BidRepository, FreelancerProfileRepository, ProjectRepository};

pub type ProjectId = Uuid;
pub type BidId = Uuid;

#[derive(Clone)]
pub struct FreelanceService {
    project_repo: ProjectRepository,
    bid_repo: BidRepository,
    profile_repo: FreelancerProfileRepository,
}

impl FreelanceService {
    pub fn new(pool: PgPool) -> Self {
        Self {
            project_repo: ProjectRepository::new(pool.clone()),
            bid_repo: BidRepository::new(pool.clone()),
            profile_repo: FreelancerProfileRepository::new(pool),
        }
    }

    pub async fn list_projects(
        &self,
        limit: i64,
        offset: i64,
        project_type: Option<ProjectType>,
        category_ids: Option<Vec<Uuid>>,
    ) -> Result<Vec<Project>, DomainError> {
        let limit = limit.clamp(1, 100);
        let offset = offset.max(0);
        self.project_repo
            .list(limit, offset, project_type, category_ids)
            .await
    }

    pub async fn get_project(&self, id: ProjectId) -> Result<Project, DomainError> {
        self.project_repo.get(id).await
    }

    pub async fn create_project(
        &self,
        req: CreateProjectRequest,
        _client_id: UserId,
    ) -> Result<Project, DomainError> {
        validate_create_project(&req)?;
        let project = self.project_repo.create(&req).await?;
        tracing::info!(
            project_id = %project.id,
            project_type = project.project_type.as_str(),
            "project created"
        );
        Ok(project)
    }

    pub async fn list_bids(
        &self,
        project_id: ProjectId,
        limit: i64,
        offset: i64,
        status: Option<BidStatus>,
    ) -> Result<Vec<Bid>, DomainError> {
        let limit = limit.clamp(1, 50);
        let offset = offset.max(0);
        self.bid_repo.list(project_id, limit, offset, status).await
    }

    pub async fn submit_bid(
        &self,
        project_id: ProjectId,
        req: CreateBidRequest,
        freelancer_id: UserId,
    ) -> Result<Bid, DomainError> {
        validate_bid(&req)?;

        let project = self.project_repo.get(project_id).await?;
        if let Some(bidding_end) = project.bidding_end_at {
            if bidding_end < chrono::Utc::now() {
                return Err(DomainError::Validation(
                    "Project bidding has ended".to_string(),
                ));
            }
        }

        if self
            .bid_repo
            .has_bid(project_id, freelancer_id)
            .await
            .unwrap_or(false)
        {
            return Err(DomainError::Conflict(
                "You have already submitted a bid for this project".to_string(),
            ));
        }

        let bid = self
            .bid_repo
            .create(project_id, freelancer_id, &req)
            .await?;
        tracing::info!(
            bid_id = %bid.id,
            project_id = %project_id,
            freelancer_id = %freelancer_id,
            "bid submitted"
        );
        Ok(bid)
    }

    pub async fn list_my_bids(
        &self,
        freelancer_id: UserId,
        limit: i64,
        offset: i64,
        status: Option<BidStatus>,
    ) -> Result<Vec<Bid>, DomainError> {
        let limit = limit.clamp(1, 50);
        let offset = offset.max(0);
        self.bid_repo
            .list_by_freelancer(freelancer_id, limit, offset, status)
            .await
    }

    pub async fn update_bid_status(
        &self,
        bid_id: BidId,
        status: BidStatus,
        _client_id: UserId,
    ) -> Result<Bid, DomainError> {
        let _bid = self.bid_repo.get(bid_id).await?;
        let updated_bid = self.bid_repo.update_status(bid_id, status).await?;
        tracing::info!(
            bid_id = %bid_id,
            status = status.as_str(),
            "bid status updated"
        );
        Ok(updated_bid)
    }

    pub async fn get_freelancer_profile(
        &self,
        candidate_profile_id: UserId,
    ) -> Result<Option<FreelancerProfile>, DomainError> {
        self.profile_repo
            .get_by_candidate_id(candidate_profile_id)
            .await
    }

    pub async fn upsert_freelancer_profile(
        &self,
        candidate_profile_id: UserId,
        req: UpsertFreelancerProfileRequest,
    ) -> Result<FreelancerProfile, DomainError> {
        validate_freelancer_profile(&req)?;
        let profile = self.profile_repo.upsert(candidate_profile_id, &req).await?;
        tracing::info!(
            candidate_profile_id = %candidate_profile_id,
            profile_id = %profile.id,
            "freelancer profile updated"
        );
        Ok(profile)
    }

    pub async fn list_categories(&self) -> Result<Vec<ProjectCategory>, DomainError> {
        self.project_repo.list_categories().await
    }
}

fn validate_create_project(req: &CreateProjectRequest) -> Result<(), DomainError> {
    if req.title.trim().is_empty() {
        return Err(DomainError::Validation("Title is required".to_string()));
    }
    if req.description.trim().is_empty() {
        return Err(DomainError::Validation(
            "Description is required".to_string(),
        ));
    }
    if let (Some(min), Some(max)) = (req.budget_min, req.budget_max) {
        if min > max {
            return Err(DomainError::Validation(
                "Minimum budget cannot be greater than maximum budget".to_string(),
            ));
        }
        if min <= rust_decimal::Decimal::ZERO {
            return Err(DomainError::Validation(
                "Budget must be greater than zero".to_string(),
            ));
        }
    }
    Ok(())
}

fn validate_bid(req: &CreateBidRequest) -> Result<(), DomainError> {
    if req.proposal.trim().is_empty() {
        return Err(DomainError::Validation("Proposal is required".to_string()));
    }
    if req.bid_amount.is_none() && req.hourly_rate.is_none() {
        return Err(DomainError::Validation(
            "Either bid amount or hourly rate must be specified".to_string(),
        ));
    }
    if let Some(amount) = req.bid_amount {
        if amount <= rust_decimal::Decimal::ZERO {
            return Err(DomainError::Validation(
                "Bid amount must be greater than zero".to_string(),
            ));
        }
    }
    if let Some(rate) = req.hourly_rate {
        if rate <= rust_decimal::Decimal::ZERO {
            return Err(DomainError::Validation(
                "Hourly rate must be greater than zero".to_string(),
            ));
        }
    }
    if let Some(hours) = req.estimated_hours {
        if hours <= 0 {
            return Err(DomainError::Validation(
                "Estimated hours must be greater than zero".to_string(),
            ));
        }
    }
    Ok(())
}

fn validate_freelancer_profile(_req: &UpsertFreelancerProfileRequest) -> Result<(), DomainError> {
    Ok(())
}
