//! Job listing domain service.

#![forbid(unsafe_code)]

mod repo;

use jobshout_domain::{CreateJobRequest, DomainError, Job, JobId, JobStatus, OrganisationId};
use sqlx::PgPool;
use uuid::Uuid;

pub use repo::JobRepository;

#[derive(Clone)]
pub struct JobService {
    repo: JobRepository,
    /// Phase 1: unauthenticated creates attach to this seed organisation.
    default_org_id: OrganisationId,
}

impl JobService {
    pub fn new(pool: PgPool, default_org_id: OrganisationId) -> Self {
        Self {
            repo: JobRepository::new(pool),
            default_org_id,
        }
    }

    pub async fn list(
        &self,
        limit: i64,
        offset: i64,
        status: Option<JobStatus>,
    ) -> Result<Vec<Job>, DomainError> {
        let limit = limit.clamp(1, 100);
        let offset = offset.max(0);
        self.repo.list(limit, offset, status).await
    }

    pub async fn get(&self, id: JobId) -> Result<Job, DomainError> {
        self.repo.get(id).await
    }

    pub async fn create(&self, req: CreateJobRequest) -> Result<Job, DomainError> {
        validate_create(&req)?;
        let status = if req.publish {
            JobStatus::Published
        } else {
            JobStatus::Draft
        };
        self.repo.create(self.default_org_id, req, status).await
    }

    pub fn default_org_id(&self) -> OrganisationId {
        self.default_org_id
    }
}

fn validate_create(req: &CreateJobRequest) -> Result<(), DomainError> {
    if req.title.trim().is_empty() {
        return Err(DomainError::Validation("title is required".into()));
    }
    if req.description.trim().is_empty() {
        return Err(DomainError::Validation("description is required".into()));
    }
    if req.location.country.trim().is_empty() {
        return Err(DomainError::Validation(
            "location.country is required".into(),
        ));
    }
    Ok(())
}

/// Stable seed organisation id used by migrations + API until auth lands.
pub fn seed_organisation_id() -> OrganisationId {
    Uuid::parse_str("11111111-1111-1111-1111-111111111111").expect("valid uuid")
}
