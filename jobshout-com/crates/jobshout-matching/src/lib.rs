//! Explainable job ↔ candidate matching for Career agents and the board UI.

#![forbid(unsafe_code)]

use jobshout_domain::{CandidateProfile, Job, JobMatch};

/// Rank published jobs for a candidate profile (0–100 scores + reasons).
pub fn rank_jobs(profile: &CandidateProfile, jobs: &[Job], limit: usize) -> Vec<JobMatch> {
    let mut scored: Vec<JobMatch> = jobs
        .iter()
        .map(|job| score_job(profile, job))
        .filter(|m| m.score > 0)
        .collect();
    scored.sort_by(|a, b| {
        b.score
            .cmp(&a.score)
            .then_with(|| a.job.title.cmp(&b.job.title))
    });
    scored.truncate(limit.max(1));
    scored
}

fn score_job(profile: &CandidateProfile, job: &Job) -> JobMatch {
    let mut score: u32 = 0;
    let mut reasons = Vec::new();

    let skill_hits = overlap_count(&profile.skills, &job.requirements);
    if skill_hits > 0 {
        let pts = (skill_hits * 12).min(40);
        score += pts;
        reasons.push(format!(
            "Matched {skill_hits} skill(s) with the role requirements (+{pts})"
        ));
    }

    let haystack = format!(
        "{} {} {}",
        job.title.to_lowercase(),
        job.summary.to_lowercase(),
        job.description.to_lowercase()
    );
    let role_hits = profile
        .preferred_roles
        .iter()
        .filter(|r| {
            let r = r.to_lowercase();
            !r.is_empty() && haystack.contains(&r)
        })
        .count() as u32;
    if role_hits > 0 {
        let pts = (role_hits * 10).min(25);
        score += pts;
        reasons.push(format!(
            "Preferred role keywords appear in the job (+{pts})"
        ));
    }

    // Soft signal from summary / CV / notes text vs requirements
    let narrative = format!(
        "{} {} {} {}",
        profile.summary, profile.cv_text, profile.headline, profile.matching_notes
    )
    .to_lowercase();
    let narrative_hits = job
        .requirements
        .iter()
        .filter(|req| {
            let req = req.to_lowercase();
            !req.is_empty() && narrative.contains(&req)
        })
        .count() as u32;
    if narrative_hits > 0 {
        let pts = (narrative_hits * 4).min(16);
        score += pts;
        reasons.push(format!(
            "Your profile text mentions {narrative_hits} job requirement(s) (+{pts})"
        ));
    }

    if profile.open_to_remote && job.location.remote {
        score += 12;
        reasons.push("Remote-friendly match (+12)".into());
    } else if location_overlap(profile, job) {
        score += 15;
        reasons.push("Location preference aligns (+15)".into());
    }

    if !profile.preferred_employment_types.is_empty()
        && profile
            .preferred_employment_types
            .contains(&job.employment_type)
    {
        score += 10;
        reasons.push(format!(
            "Employment type {} matches preference (+10)",
            job.employment_type.as_str()
        ));
    }

    if !profile.matching_notes.trim().is_empty() {
        score = score.saturating_add(3);
        reasons.push("Matching notes present for agent context (+3)".into());
    }

    let score = score.min(100) as u8;
    if reasons.is_empty() && score == 0 {
        reasons.push("No strong overlap yet — broaden skills or preferred roles".into());
    }

    JobMatch {
        job: job.clone(),
        score,
        reasons,
    }
}

fn overlap_count(a: &[String], b: &[String]) -> u32 {
    a.iter()
        .filter(|x| {
            b.iter()
                .any(|y| x.eq_ignore_ascii_case(y) || y.to_lowercase().contains(&x.to_lowercase()))
        })
        .count() as u32
}

fn location_overlap(profile: &CandidateProfile, job: &Job) -> bool {
    if profile.preferred_locations.is_empty() {
        return false;
    }
    profile.preferred_locations.iter().any(|pref| {
        let country_ok = pref.country.eq_ignore_ascii_case(&job.location.country);
        let city_ok = match (&pref.city, &job.location.city) {
            (Some(a), Some(b)) => a.eq_ignore_ascii_case(b),
            (None, _) => true,
            _ => false,
        };
        country_ok && city_ok
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;
    use jobshout_domain::{Compensation, EmploymentType, JobStatus, Location};
    use uuid::Uuid;

    fn sample_profile() -> CandidateProfile {
        CandidateProfile {
            id: Uuid::new_v4(),
            email: "ada@example.com".into(),
            display_name: "Ada".into(),
            headline: "Rust engineer".into(),
            summary: "Built Axum APIs and PostgreSQL services.".into(),
            skills: vec!["Rust".into(), "Axum".into(), "PostgreSQL".into()],
            years_experience: Some(8),
            preferred_roles: vec!["Rust Engineer".into()],
            preferred_locations: vec![Location {
                country: "GB".into(),
                region: None,
                city: Some("London".into()),
                remote: true,
            }],
            preferred_employment_types: vec![EmploymentType::Permanent],
            open_to_remote: true,
            salary_expectation: Compensation::default(),
            cv_text: String::new(),
            matching_notes: "Prefer deep systems work".into(),
            created_at: Utc::now(),
            updated_at: Utc::now(),
        }
    }

    fn sample_job(title: &str, requirements: &[&str], remote: bool) -> Job {
        Job {
            id: Uuid::new_v4(),
            organisation_id: Uuid::new_v4(),
            title: title.into(),
            summary: "Build marketplace services".into(),
            description: "Rust and Axum".into(),
            employment_type: EmploymentType::Permanent,
            location: Location {
                country: "GB".into(),
                region: None,
                city: Some("London".into()),
                remote,
            },
            compensation: Compensation::default(),
            requirements: requirements.iter().map(|s| (*s).to_string()).collect(),
            status: JobStatus::Published,
            created_at: Utc::now(),
            updated_at: Utc::now(),
            published_at: Some(Utc::now()),
        }
    }

    #[test]
    fn ranks_rust_role_highly() {
        let profile = sample_profile();
        let jobs = vec![
            sample_job(
                "Senior Rust Engineer",
                &["Rust", "Tokio", "Axum", "PostgreSQL"],
                true,
            ),
            sample_job("Technical Writer", &["Technical writing"], false),
        ];
        let ranked = rank_jobs(&profile, &jobs, 5);
        assert!(!ranked.is_empty());
        assert_eq!(ranked[0].job.title, "Senior Rust Engineer");
        assert!(ranked[0].score >= 50);
    }
}
