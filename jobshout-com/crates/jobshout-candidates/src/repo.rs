use chrono::{DateTime, Utc};
use jobshout_domain::{
    CandidateProfile, CandidateProfileId, Compensation, DomainError, EmploymentType, Location,
    UpsertCandidateProfileRequest,
};
use sqlx::{PgPool, Row};
use uuid::Uuid;

#[derive(Clone)]
pub struct CandidateRepository {
    pool: PgPool,
}

impl CandidateRepository {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn get(&self, id: CandidateProfileId) -> Result<CandidateProfile, DomainError> {
        let row = sqlx::query(
            r#"
            SELECT id, email, display_name, headline, summary, skills, years_experience,
                   preferred_roles, preferred_locations, preferred_employment_types,
                   open_to_remote, salary_expectation, cv_text, matching_notes,
                   created_at, updated_at
            FROM candidate_profiles
            WHERE id = $1
            "#,
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        match row {
            Some(r) => map_row(r),
            None => Err(DomainError::NotFound),
        }
    }

    pub async fn get_by_email(&self, email: &str) -> Result<CandidateProfile, DomainError> {
        let row = sqlx::query(
            r#"
            SELECT id, email, display_name, headline, summary, skills, years_experience,
                   preferred_roles, preferred_locations, preferred_employment_types,
                   open_to_remote, salary_expectation, cv_text, matching_notes,
                   created_at, updated_at
            FROM candidate_profiles
            WHERE lower(email) = lower($1)
            "#,
        )
        .bind(email)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        match row {
            Some(r) => map_row(r),
            None => Err(DomainError::NotFound),
        }
    }

    pub async fn upsert(
        &self,
        req: UpsertCandidateProfileRequest,
    ) -> Result<CandidateProfile, DomainError> {
        let now = Utc::now();
        let id = Uuid::new_v4();

        let skills = serde_json::to_value(&req.skills).map_err(|e| DomainError::Other(e.into()))?;
        let preferred_roles =
            serde_json::to_value(&req.preferred_roles).map_err(|e| DomainError::Other(e.into()))?;
        let preferred_locations = serde_json::to_value(&req.preferred_locations)
            .map_err(|e| DomainError::Other(e.into()))?;
        let preferred_employment_types: Vec<String> = req
            .preferred_employment_types
            .iter()
            .map(|e| e.as_str().to_string())
            .collect();
        let preferred_employment_types = serde_json::to_value(&preferred_employment_types)
            .map_err(|e| DomainError::Other(e.into()))?;
        let salary = serde_json::to_value(&req.salary_expectation)
            .map_err(|e| DomainError::Other(e.into()))?;

        let row = sqlx::query(
            r#"
            INSERT INTO candidate_profiles (
              id, email, display_name, headline, summary, skills, years_experience,
              preferred_roles, preferred_locations, preferred_employment_types,
              open_to_remote, salary_expectation, cv_text, matching_notes,
              created_at, updated_at
            ) VALUES (
              $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16
            )
            ON CONFLICT (email) DO UPDATE SET
              display_name = EXCLUDED.display_name,
              headline = EXCLUDED.headline,
              summary = EXCLUDED.summary,
              skills = EXCLUDED.skills,
              years_experience = EXCLUDED.years_experience,
              preferred_roles = EXCLUDED.preferred_roles,
              preferred_locations = EXCLUDED.preferred_locations,
              preferred_employment_types = EXCLUDED.preferred_employment_types,
              open_to_remote = EXCLUDED.open_to_remote,
              salary_expectation = EXCLUDED.salary_expectation,
              cv_text = EXCLUDED.cv_text,
              matching_notes = EXCLUDED.matching_notes,
              updated_at = EXCLUDED.updated_at
            RETURNING id
            "#,
        )
        .bind(id)
        .bind(&req.email)
        .bind(req.display_name.trim())
        .bind(req.headline.trim())
        .bind(req.summary.trim())
        .bind(skills)
        .bind(req.years_experience)
        .bind(preferred_roles)
        .bind(preferred_locations)
        .bind(preferred_employment_types)
        .bind(req.open_to_remote)
        .bind(salary)
        .bind(req.cv_text.trim())
        .bind(req.matching_notes.trim())
        .bind(now)
        .bind(now)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        let id: Uuid = row.get("id");
        self.get(id).await
    }
}

fn map_row(row: sqlx::postgres::PgRow) -> Result<CandidateProfile, DomainError> {
    let skills: Vec<String> =
        serde_json::from_value(row.get("skills")).map_err(|e| DomainError::Other(e.into()))?;
    let preferred_roles: Vec<String> = serde_json::from_value(row.get("preferred_roles"))
        .map_err(|e| DomainError::Other(e.into()))?;
    let preferred_locations: Vec<Location> = serde_json::from_value(row.get("preferred_locations"))
        .map_err(|e| DomainError::Other(e.into()))?;
    let employment_raw: Vec<String> = serde_json::from_value(row.get("preferred_employment_types"))
        .map_err(|e| DomainError::Other(e.into()))?;
    let preferred_employment_types = employment_raw
        .into_iter()
        .filter_map(|s| EmploymentType::parse(&s))
        .collect();
    let salary_expectation: Compensation = serde_json::from_value(row.get("salary_expectation"))
        .map_err(|e| DomainError::Other(e.into()))?;

    Ok(CandidateProfile {
        id: row.get("id"),
        email: row.get("email"),
        display_name: row.get("display_name"),
        headline: row.get("headline"),
        summary: row.get("summary"),
        skills,
        years_experience: row.get("years_experience"),
        preferred_roles,
        preferred_locations,
        preferred_employment_types,
        open_to_remote: row.get("open_to_remote"),
        salary_expectation,
        cv_text: row.get("cv_text"),
        matching_notes: row.get("matching_notes"),
        created_at: row.get::<DateTime<Utc>, _>("created_at"),
        updated_at: row.get::<DateTime<Utc>, _>("updated_at"),
    })
}
