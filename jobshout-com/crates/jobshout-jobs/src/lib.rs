//! Job listing domain service.

#![forbid(unsafe_code)]

mod repo;
mod saved;

use jobshout_domain::{CreateJobRequest, DomainError, Job, JobId, JobStatus, OrganisationId};
use sqlx::PgPool;
use uuid::Uuid;

pub use repo::JobSearch;
pub use repo::{job_from_row, JobRepository};
pub use saved::{JobAlert, JobAlertInput, SavedJobs};

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

    pub async fn search(
        &self,
        status: Option<JobStatus>,
        filters: &JobSearch,
        limit: i64,
        offset: i64,
    ) -> Result<(Vec<Job>, i64), DomainError> {
        self.repo
            .search(status, filters, limit.clamp(1, 100), offset.max(0))
            .await
    }

    /// Jobs posted by `poster_email`, every status, so drafts are findable.
    pub async fn list_mine(&self, poster_email: &str) -> Result<Vec<Job>, DomainError> {
        if poster_email.trim().is_empty() {
            return Ok(Vec::new());
        }
        self.repo.list_by_poster(poster_email, 100).await
    }

    pub async fn publish(&self, id: JobId) -> Result<Job, DomainError> {
        self.repo.publish(id).await
    }

    pub async fn get(&self, id: JobId) -> Result<Job, DomainError> {
        self.repo.get(id).await
    }

    /// `poster_email` is the signed-in user posting it. It decides who may
    /// link the job from the AI Showcase and who may see applications; it is
    /// never returned by the jobs API.
    pub async fn create(
        &self,
        req: CreateJobRequest,
        poster_email: &str,
    ) -> Result<Job, DomainError> {
        validate_create(&req)?;
        let poster = poster_email.trim();
        if poster.is_empty() {
            return Err(DomainError::Unauthorized("sign in to post a job".into()));
        }
        let status = if req.publish {
            JobStatus::Published
        } else {
            JobStatus::Draft
        };
        self.repo
            .create(self.default_org_id, req, status, poster)
            .await
    }

    pub async fn poster_email(&self, id: JobId) -> Result<String, DomainError> {
        self.repo.poster_email(id).await
    }

    pub async fn update(&self, id: JobId, req: CreateJobRequest) -> Result<Job, DomainError> {
        validate_create(&req)?;
        // Editing must never reopen a closed role as a side effect of
        // `publish: true`; closed and archived jobs are read-only.
        let current = self.repo.get(id).await?;
        if !editable(current.status) {
            return Err(DomainError::Validation(format!(
                "a {} job cannot be edited",
                current.status.as_str()
            )));
        }
        self.repo.update(id, req).await
    }

    /// Delete a draft. Live and closed jobs are closed instead, so no
    /// candidate's application is ever removed with them.
    pub async fn delete_draft(&self, id: JobId) -> Result<(), DomainError> {
        self.repo.delete_draft(id).await
    }

    pub async fn close(&self, id: JobId) -> Result<Job, DomainError> {
        self.repo.close(id).await
    }

    pub fn default_org_id(&self) -> OrganisationId {
        self.default_org_id
    }
}

/// Drafts and live jobs can be edited; closed and archived ones cannot.
pub fn editable(status: JobStatus) -> bool {
    matches!(status, JobStatus::Draft | JobStatus::Published)
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

#[cfg(test)]
mod tests {
    use super::*;
    use jobshout_domain::{EmploymentType, Location};

    fn req() -> CreateJobRequest {
        CreateJobRequest {
            title: "Engineer".into(),
            summary: String::new(),
            description: "Build things.".into(),
            employment_type: EmploymentType::Permanent,
            location: Location {
                country: "GB".into(),
                region: None,
                city: None,
                remote: true,
            },
            compensation: Default::default(),
            requirements: vec![],
            publish: false,
        }
    }

    #[test]
    fn only_drafts_and_live_jobs_are_editable() {
        assert!(editable(JobStatus::Draft));
        assert!(editable(JobStatus::Published));
        assert!(!editable(JobStatus::Closed));
        assert!(!editable(JobStatus::Archived));
    }

    #[test]
    fn validation_requires_title_description_country() {
        assert!(validate_create(&req()).is_ok());
        let mut r = req();
        r.title = "  ".into();
        assert!(validate_create(&r).is_err());
        let mut r = req();
        r.description = String::new();
        assert!(validate_create(&r).is_err());
        let mut r = req();
        r.location.country = String::new();
        assert!(validate_create(&r).is_err());
    }
}
