//! Application state machine.
//!
//! Candidates apply to published jobs. Re-applying to the same job with the
//! same email updates the existing application rather than creating a duplicate.

#![forbid(unsafe_code)]

mod repo;

use jobshout_domain::{
    CreateJobApplicationRequest, DomainError, JobApplication, JobApplicationWithJob, JobId,
};
use sqlx::PgPool;

pub use repo::ApplicationRepository;

#[derive(Clone)]
pub struct ApplicationService {
    repo: ApplicationRepository,
}

impl ApplicationService {
    pub fn new(pool: PgPool) -> Self {
        Self {
            repo: ApplicationRepository::new(pool),
        }
    }

    /// Submit (or re-submit) an application for a job.
    pub async fn apply(
        &self,
        job_id: JobId,
        req: CreateJobApplicationRequest,
    ) -> Result<JobApplication, DomainError> {
        let req = validate(req)?;
        self.repo.upsert(job_id, req).await
    }

    pub async fn list_for_job(
        &self,
        job_id: JobId,
        limit: i64,
        offset: i64,
    ) -> Result<Vec<JobApplication>, DomainError> {
        self.repo
            .list_for_job(job_id, limit.clamp(1, 100), offset.max(0))
            .await
    }

    /// Every application a candidate has sent, newest first, with the job attached.
    pub async fn list_for_email(
        &self,
        email: &str,
        limit: i64,
    ) -> Result<Vec<JobApplicationWithJob>, DomainError> {
        let email = email.trim();
        if email.is_empty() {
            return Err(DomainError::Validation("email is required".into()));
        }
        self.repo.list_for_email(email, limit.clamp(1, 100)).await
    }

    /// Has this candidate already applied to this job?
    pub async fn find(
        &self,
        job_id: JobId,
        email: &str,
    ) -> Result<Option<JobApplication>, DomainError> {
        self.repo.find(job_id, email.trim()).await
    }

    pub async fn count_for_job(&self, job_id: JobId) -> Result<i64, DomainError> {
        self.repo.count_for_job(job_id).await
    }
}

fn validate(
    mut req: CreateJobApplicationRequest,
) -> Result<CreateJobApplicationRequest, DomainError> {
    req.full_name = req.full_name.trim().to_string();
    req.email = req.email.trim().to_lowercase();
    req.phone = req.phone.trim().to_string();
    req.cover_letter = req.cover_letter.trim().to_string();
    req.cv_text = req.cv_text.trim().to_string();
    req.cv_url = req.cv_url.trim().to_string();

    if req.full_name.is_empty() {
        return Err(DomainError::Validation("full_name is required".into()));
    }
    if req.email.is_empty() {
        return Err(DomainError::Validation("email is required".into()));
    }
    if !req.email.contains('@') || req.email.starts_with('@') || req.email.ends_with('@') {
        return Err(DomainError::Validation("email is not valid".into()));
    }
    if req.cover_letter.is_empty() && req.cv_text.is_empty() && req.cv_url.is_empty() {
        return Err(DomainError::Validation(
            "add a cover letter, CV text, or a CV link".into(),
        ));
    }
    Ok(req)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn req() -> CreateJobApplicationRequest {
        CreateJobApplicationRequest {
            full_name: "  Ada Lovelace ".into(),
            email: " Ada@Example.COM ".into(),
            phone: String::new(),
            cover_letter: "I would love this role.".into(),
            cv_text: String::new(),
            cv_url: String::new(),
        }
    }

    #[test]
    fn normalises_name_and_email() {
        let out = validate(req()).expect("valid");
        assert_eq!(out.full_name, "Ada Lovelace");
        assert_eq!(out.email, "ada@example.com");
    }

    #[test]
    fn rejects_missing_name() {
        let mut r = req();
        r.full_name = "   ".into();
        assert!(validate(r).is_err());
    }

    #[test]
    fn rejects_bad_email() {
        let mut r = req();
        r.email = "not-an-email".into();
        assert!(validate(r).is_err());
    }

    #[test]
    fn rejects_empty_application_body() {
        let mut r = req();
        r.cover_letter = String::new();
        assert!(validate(r).is_err());
    }

    #[test]
    fn cv_url_alone_is_enough() {
        let mut r = req();
        r.cover_letter = String::new();
        r.cv_url = "https://example.com/cv.pdf".into();
        assert!(validate(r).is_ok());
    }
}
