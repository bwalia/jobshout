//! Candidate profile persistence and validation.

#![forbid(unsafe_code)]

mod repo;

use jobshout_domain::{
    CandidateProfile, CandidateProfileId, DomainError, UpsertCandidateProfileRequest,
};
use sqlx::PgPool;

pub use repo::CandidateRepository;

#[derive(Clone)]
pub struct CandidateService {
    repo: CandidateRepository,
}

impl CandidateService {
    pub fn new(pool: PgPool) -> Self {
        Self {
            repo: CandidateRepository::new(pool),
        }
    }

    pub async fn get(&self, id: CandidateProfileId) -> Result<CandidateProfile, DomainError> {
        self.repo.get(id).await
    }

    pub async fn get_by_email(&self, email: &str) -> Result<CandidateProfile, DomainError> {
        let email = normalize_email(email)?;
        self.repo.get_by_email(&email).await
    }

    pub async fn upsert(
        &self,
        req: UpsertCandidateProfileRequest,
    ) -> Result<CandidateProfile, DomainError> {
        validate_upsert(&req)?;
        let mut req = req;
        req.email = normalize_email(&req.email)?;
        req.display_name = req.display_name.trim().to_string();
        req.skills = clean_tags(req.skills);
        req.preferred_roles = clean_tags(req.preferred_roles);
        self.repo.upsert(req).await
    }
}

fn normalize_email(email: &str) -> Result<String, DomainError> {
    let email = email.trim().to_lowercase();
    if email.is_empty() || !email.contains('@') || email.len() > 320 {
        return Err(DomainError::Validation("a valid email is required".into()));
    }
    Ok(email)
}

fn clean_tags(tags: Vec<String>) -> Vec<String> {
    let mut out = Vec::new();
    for t in tags {
        let t = t.trim().to_string();
        if t.is_empty() {
            continue;
        }
        if !out.iter().any(|x: &String| x.eq_ignore_ascii_case(&t)) {
            out.push(t);
        }
    }
    out
}

fn validate_upsert(req: &UpsertCandidateProfileRequest) -> Result<(), DomainError> {
    if req.display_name.trim().is_empty() {
        return Err(DomainError::Validation("display_name is required".into()));
    }
    if req.skills.is_empty() && req.summary.trim().is_empty() && req.cv_text.trim().is_empty() {
        return Err(DomainError::Validation(
            "add skills, a summary, or CV text so agents can match you".into(),
        ));
    }
    if let Some(y) = req.years_experience {
        if !(0..=60).contains(&y) {
            return Err(DomainError::Validation(
                "years_experience must be between 0 and 60".into(),
            ));
        }
    }
    Ok(())
}
