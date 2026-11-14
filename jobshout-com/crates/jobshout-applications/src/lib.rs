//! Application state machine.
//!
//! Candidates apply to published jobs. Re-applying to the same job with the
//! same email updates the existing application rather than creating a duplicate.

#![forbid(unsafe_code)]

mod repo;

use jobshout_domain::{
    ApplicationStatus, CreateJobApplicationRequest, DomainError, JobApplication, JobApplicationId,
    JobApplicationWithJob, JobId,
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

    /// The hiring side moves an application through the pipeline. Only
    /// stages a recruiter can set are accepted, and a withdrawn application
    /// is final.
    pub async fn set_status(
        &self,
        job_id: JobId,
        application_id: JobApplicationId,
        status: ApplicationStatus,
    ) -> Result<JobApplication, DomainError> {
        if !recruiter_can_set(status) {
            return Err(DomainError::Validation(format!(
                "{} cannot be set by the hiring team",
                status.as_str()
            )));
        }
        self.repo.set_status(job_id, application_id, status).await
    }

    /// The candidate withdraws their own application.
    pub async fn withdraw(
        &self,
        application_id: JobApplicationId,
        email: &str,
    ) -> Result<JobApplication, DomainError> {
        self.repo.withdraw(application_id, email.trim()).await
    }
}

/// Pipeline stages the hiring team may move an application to. `withdrawn`
/// belongs to the candidate alone.
pub fn recruiter_can_set(status: ApplicationStatus) -> bool {
    !matches!(status, ApplicationStatus::Withdrawn)
}

const MAX_NAME: usize = 200;
const MAX_PHONE: usize = 50;
const MAX_URL: usize = 2_000;
const MAX_COVER_LETTER: usize = 20_000;
const MAX_CV_TEXT: usize = 60_000;

fn too_long(field: &str, value: &str, max: usize) -> Result<(), DomainError> {
    if value.chars().count() > max {
        return Err(DomainError::Validation(format!(
            "{field} is too long (max {max} characters)"
        )));
    }
    Ok(())
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
    too_long("full_name", &req.full_name, MAX_NAME)?;
    too_long("phone", &req.phone, MAX_PHONE)?;
    too_long("cv_url", &req.cv_url, MAX_URL)?;
    too_long("cover_letter", &req.cover_letter, MAX_COVER_LETTER)?;
    too_long("cv_text", &req.cv_text, MAX_CV_TEXT)?;
    // The link is shown to recruiters, so only plain web links are stored
    // (no javascript:, data: or file: URLs).
    if !req.cv_url.is_empty() {
        let lower = req.cv_url.to_ascii_lowercase();
        if !(lower.starts_with("https://") || lower.starts_with("http://")) {
            return Err(DomainError::Validation(
                "cv_url must be an http(s) link".into(),
            ));
        }
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
    fn rejects_script_and_data_cv_links() {
        for bad in [
            "javascript:alert(1)",
            "data:text/html,x",
            "file:///etc/passwd",
            "ftp://x",
        ] {
            let mut r = req();
            r.cv_url = bad.into();
            assert!(validate(r).is_err(), "{bad} should be rejected");
        }
    }

    #[test]
    fn rejects_oversized_fields() {
        let mut r = req();
        r.cover_letter = "x".repeat(MAX_COVER_LETTER + 1);
        assert!(validate(r).is_err());
        let mut r = req();
        r.phone = "1".repeat(MAX_PHONE + 1);
        assert!(validate(r).is_err());
    }

    #[test]
    fn recruiters_cannot_withdraw_for_the_candidate() {
        assert!(!recruiter_can_set(ApplicationStatus::Withdrawn));
        assert!(recruiter_can_set(ApplicationStatus::Shortlisted));
        assert!(recruiter_can_set(ApplicationStatus::Rejected));
    }

    #[test]
    fn cv_url_alone_is_enough() {
        let mut r = req();
        r.cover_letter = String::new();
        r.cv_url = "https://example.com/cv.pdf".into();
        assert!(validate(r).is_ok());
    }
}
