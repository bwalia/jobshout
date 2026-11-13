//! Privacy helpers: sensitive fields only serialise for owners / hirers.

#![allow(dead_code)] // exercised by unit tests; handy for future non-owner views

use jobshout_domain::{CandidateProfile, JobApplication, JobApplicationWithJob};
use serde::Serialize;
use serde_json::{json, Value};

/// Profile JSON for the owner (or a trusted matching agent).
pub fn profile_owner_json(p: &CandidateProfile) -> Value {
    serde_json::to_value(p).unwrap_or_else(|_| json!({}))
}

/// Profile JSON with email / CV / salary stripped. Used in tests and any
/// future non-owner view; live endpoints return 403 instead.
pub fn profile_public_json(p: &CandidateProfile) -> Value {
    json!({
        "id": p.id,
        "display_name": p.display_name,
        "headline": p.headline,
        "summary": p.summary,
        "skills": p.skills,
        "years_experience": p.years_experience,
        "preferred_roles": p.preferred_roles,
        "preferred_locations": p.preferred_locations,
        "preferred_employment_types": p.preferred_employment_types,
        "open_to_remote": p.open_to_remote,
        "matching_notes": p.matching_notes,
        "created_at": p.created_at,
        "updated_at": p.updated_at,
    })
}

/// Application as the applicant or the job poster sees it (full PII).
pub fn application_owner_json(a: &JobApplication) -> Value {
    serde_json::to_value(a).unwrap_or_else(|_| json!({}))
}

/// Application with email / phone / CV removed.
pub fn application_redacted_json(a: &JobApplication) -> Value {
    json!({
        "id": a.id,
        "job_id": a.job_id,
        "candidate_profile_id": a.candidate_profile_id,
        "full_name": a.full_name,
        "cover_letter": a.cover_letter,
        "status": a.status,
        "created_at": a.created_at,
        "updated_at": a.updated_at,
    })
}

pub fn applications_with_job_owner_json(items: &[JobApplicationWithJob]) -> Value {
    #[derive(Serialize)]
    struct Row<'a> {
        #[serde(flatten)]
        application: &'a JobApplication,
        job: &'a jobshout_domain::Job,
    }
    let data: Vec<_> = items
        .iter()
        .map(|i| Row {
            application: &i.application,
            job: &i.job,
        })
        .collect();
    json!({ "data": data })
}

/// True when a serialised profile JSON contains no private keys.
pub fn profile_json_is_redacted(v: &Value) -> bool {
    v.get("email").is_none()
        && v.get("cv_text").is_none()
        && v.get("salary_expectation").is_none()
        && v.get("phone").is_none()
}

/// True when a serialised application JSON contains no private keys.
pub fn application_json_is_redacted(v: &Value) -> bool {
    v.get("email").is_none()
        && v.get("phone").is_none()
        && v.get("cv_text").is_none()
        && v.get("cv_url").is_none()
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;
    use jobshout_domain::{ApplicationStatus, Compensation, JobApplication};
    use uuid::Uuid;

    fn sample_app() -> JobApplication {
        JobApplication {
            id: Uuid::new_v4(),
            job_id: Uuid::new_v4(),
            candidate_profile_id: None,
            full_name: "Ada".into(),
            email: format!("{}@{}", "secret", "example.com"),
            phone: format!("{}{}", "+44", "7700"),
            cover_letter: "hi".into(),
            cv_text: "CV PRIVATE".into(),
            cv_url: "https://example.com/cv.pdf".into(),
            status: ApplicationStatus::Submitted,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        }
    }

    #[test]
    fn redacted_application_omits_pii() {
        let a = sample_app();
        let v = application_redacted_json(&a);
        assert!(application_json_is_redacted(&v));
        assert_eq!(v["full_name"], "Ada");
        let owner = application_owner_json(&a);
        assert!(owner.get("email").is_some());
        assert!(owner.get("phone").is_some());
        assert!(owner.get("cv_text").is_some());
    }

    #[test]
    fn redacted_profile_omits_pii() {
        let p = CandidateProfile {
            id: Uuid::new_v4(),
            email: format!("{}@{}", "ada", "example.com"),
            display_name: "Ada".into(),
            headline: "".into(),
            summary: "".into(),
            skills: vec![],
            years_experience: None,
            preferred_roles: vec![],
            preferred_locations: vec![],
            preferred_employment_types: vec![],
            open_to_remote: true,
            salary_expectation: Compensation::default(),
            cv_text: "PRIVATE".into(),
            matching_notes: "".into(),
            created_at: Utc::now(),
            updated_at: Utc::now(),
        };
        let v = profile_public_json(&p);
        assert!(profile_json_is_redacted(&v));
        assert!(!profile_json_is_redacted(&profile_owner_json(&p)));
    }
}
