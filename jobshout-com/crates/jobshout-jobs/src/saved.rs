//! Saved jobs and job alerts. Both belong to one account and are only ever
//! read or changed by that account.

use chrono::{DateTime, Utc};
use jobshout_domain::{DomainError, Job, JobId};
use serde::{Deserialize, Serialize};
use sqlx::{PgPool, Row};
use uuid::Uuid;

use crate::repo::job_from_row;

fn db(e: sqlx::Error) -> DomainError {
    DomainError::Other(e.into())
}

#[derive(Debug, Clone, Serialize)]
pub struct JobAlert {
    pub id: Uuid,
    pub q: String,
    pub location: String,
    pub types: Vec<String>,
    pub remote: bool,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct JobAlertInput {
    #[serde(default)]
    pub q: String,
    #[serde(default)]
    pub location: String,
    #[serde(default)]
    pub types: Vec<String>,
    #[serde(default)]
    pub remote: bool,
}

/// Most alerts one account may keep.
pub const MAX_ALERTS: i64 = 20;

#[derive(Clone)]
pub struct SavedJobs {
    pool: PgPool,
}

impl SavedJobs {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    /// Save a published job. Saving twice is a no-op.
    pub async fn save(&self, account_id: Uuid, job_id: JobId) -> Result<(), DomainError> {
        let status: Option<String> = sqlx::query_scalar("SELECT status FROM jobs WHERE id = $1")
            .bind(job_id)
            .fetch_optional(&self.pool)
            .await
            .map_err(db)?;
        if status.as_deref() != Some("published") {
            return Err(DomainError::NotFound);
        }
        sqlx::query(
            "INSERT INTO saved_jobs (account_id, job_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
        )
        .bind(account_id)
        .bind(job_id)
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(())
    }

    pub async fn unsave(&self, account_id: Uuid, job_id: JobId) -> Result<(), DomainError> {
        sqlx::query("DELETE FROM saved_jobs WHERE account_id = $1 AND job_id = $2")
            .bind(account_id)
            .bind(job_id)
            .execute(&self.pool)
            .await
            .map_err(db)?;
        Ok(())
    }

    pub async fn is_saved(&self, account_id: Uuid, job_id: JobId) -> Result<bool, DomainError> {
        sqlx::query_scalar(
            "SELECT EXISTS (SELECT 1 FROM saved_jobs WHERE account_id = $1 AND job_id = $2)",
        )
        .bind(account_id)
        .bind(job_id)
        .fetch_one(&self.pool)
        .await
        .map_err(db)
    }

    /// Saved jobs, newest save first. Jobs closed since are still listed
    /// (with their status) so the candidate knows what happened to them.
    pub async fn list(&self, account_id: Uuid) -> Result<Vec<Job>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT j.id, j.organisation_id, j.title, j.summary, j.description, j.employment_type,
                   j.location, j.compensation, j.requirements, j.status, j.created_at,
                   j.updated_at, j.published_at
            FROM saved_jobs s JOIN jobs j ON j.id = s.job_id
            WHERE s.account_id = $1 AND j.status IN ('published', 'closed')
            ORDER BY s.created_at DESC
            LIMIT 200
            "#,
        )
        .bind(account_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        rows.iter().map(job_from_row).collect()
    }

    pub async fn alerts(&self, account_id: Uuid) -> Result<Vec<JobAlert>, DomainError> {
        let rows = sqlx::query(
            "SELECT id, q, location, types, remote, created_at FROM job_alerts \
             WHERE account_id = $1 ORDER BY created_at DESC",
        )
        .bind(account_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| JobAlert {
                id: r.get("id"),
                q: r.get("q"),
                location: r.get("location"),
                types: r.get("types"),
                remote: r.get("remote"),
                created_at: r.get("created_at"),
            })
            .collect())
    }

    pub async fn create_alert(
        &self,
        account_id: Uuid,
        input: JobAlertInput,
    ) -> Result<JobAlert, DomainError> {
        let input = validate_alert(input)?;
        let count: i64 =
            sqlx::query_scalar("SELECT COUNT(*) FROM job_alerts WHERE account_id = $1")
                .bind(account_id)
                .fetch_one(&self.pool)
                .await
                .map_err(db)?;
        if count >= MAX_ALERTS {
            return Err(DomainError::Validation(format!(
                "you can keep up to {MAX_ALERTS} alerts; delete one first"
            )));
        }
        let row = sqlx::query(
            r#"
            INSERT INTO job_alerts (id, account_id, q, location, types, remote)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id, q, location, types, remote, created_at
            "#,
        )
        .bind(Uuid::new_v4())
        .bind(account_id)
        .bind(&input.q)
        .bind(&input.location)
        .bind(&input.types)
        .bind(input.remote)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| match &e {
            sqlx::Error::Database(d) if d.code().as_deref() == Some("23505") => {
                DomainError::Conflict("you already have an alert for this search".into())
            }
            _ => db(e),
        })?;
        Ok(JobAlert {
            id: row.get("id"),
            q: row.get("q"),
            location: row.get("location"),
            types: row.get("types"),
            remote: row.get("remote"),
            created_at: row.get("created_at"),
        })
    }

    /// Change one of the account's own alerts; anyone else's id is NotFound.
    pub async fn update_alert(
        &self,
        account_id: Uuid,
        id: Uuid,
        input: JobAlertInput,
    ) -> Result<JobAlert, DomainError> {
        let input = validate_alert(input)?;
        let row = sqlx::query(
            r#"
            UPDATE job_alerts SET q = $3, location = $4, types = $5, remote = $6
            WHERE id = $1 AND account_id = $2
            RETURNING id, q, location, types, remote, created_at
            "#,
        )
        .bind(id)
        .bind(account_id)
        .bind(&input.q)
        .bind(&input.location)
        .bind(&input.types)
        .bind(input.remote)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| match &e {
            sqlx::Error::Database(d) if d.code().as_deref() == Some("23505") => {
                DomainError::Conflict("you already have an alert for this search".into())
            }
            _ => db(e),
        })?
        .ok_or(DomainError::NotFound)?;
        Ok(JobAlert {
            id: row.get("id"),
            q: row.get("q"),
            location: row.get("location"),
            types: row.get("types"),
            remote: row.get("remote"),
            created_at: row.get("created_at"),
        })
    }

    /// Delete one of the account's own alerts; anyone else's id is NotFound.
    pub async fn delete_alert(&self, account_id: Uuid, id: Uuid) -> Result<(), DomainError> {
        let r = sqlx::query("DELETE FROM job_alerts WHERE id = $1 AND account_id = $2")
            .bind(id)
            .bind(account_id)
            .execute(&self.pool)
            .await
            .map_err(db)?;
        if r.rows_affected() == 0 {
            return Err(DomainError::NotFound);
        }
        Ok(())
    }

    /// Emails of accounts whose alerts match a just-published job, never
    /// including the poster. Same matching rules as the board search.
    pub async fn alert_recipients(&self, job_id: JobId) -> Result<Vec<String>, DomainError> {
        sqlx::query_scalar(
            r#"
            SELECT DISTINCT acc.primary_email
            FROM job_alerts a
            JOIN accounts acc ON acc.id = a.account_id
            JOIN jobs j ON j.id = $1
            WHERE j.status = 'published'
              AND lower(acc.primary_email) <> lower(j.poster_email)
              AND (a.q = '' OR concat_ws(' ', j.title, j.summary, j.description, j.requirements::text)
                   ILIKE '%' || a.q || '%')
              AND (a.location = ''
                   OR (lower(a.location) IN ('remote', 'anywhere')
                       AND COALESCE((j.location->>'remote')::boolean, false))
                   OR concat_ws(' ', j.location->>'city', j.location->>'region', j.location->>'country')
                      ILIKE '%' || a.location || '%')
              AND (cardinality(a.types) = 0 OR j.employment_type = ANY(a.types))
              AND (NOT a.remote OR COALESCE((j.location->>'remote')::boolean, false))
            LIMIT 5000
            "#,
        )
        .bind(job_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)
    }
}

const EMPLOYMENT_TYPES: &[&str] = &[
    "permanent",
    "contract",
    "freelance",
    "temporary",
    "part_time",
    "internship",
    "apprenticeship",
];

/// Trim, bound and check an alert. An alert with no criteria at all would
/// fire on every job, so at least one is required.
pub fn validate_alert(mut a: JobAlertInput) -> Result<JobAlertInput, DomainError> {
    // `%` and `_` would act as wildcards in the matcher; strip them.
    let clean = |s: &str| {
        s.trim()
            .chars()
            .filter(|c| *c != '%' && *c != '_' && *c != '\\')
            .collect::<String>()
    };
    a.q = clean(&a.q);
    a.location = clean(&a.location);
    a.types.retain(|t| !t.trim().is_empty());
    a.types.sort();
    a.types.dedup();
    if a.q.chars().count() > 120 || a.location.chars().count() > 120 {
        return Err(DomainError::Validation(
            "keep keywords and location under 120 characters".into(),
        ));
    }
    if let Some(bad) = a
        .types
        .iter()
        .find(|t| !EMPLOYMENT_TYPES.contains(&t.as_str()))
    {
        return Err(DomainError::Validation(format!("unknown work type: {bad}")));
    }
    if a.q.is_empty() && a.location.is_empty() && a.types.is_empty() && !a.remote {
        return Err(DomainError::Validation(
            "add a keyword, location, work type or remote to the alert".into(),
        ));
    }
    Ok(a)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_alert_is_rejected() {
        assert!(validate_alert(JobAlertInput::default()).is_err());
    }

    #[test]
    fn wildcards_are_stripped_and_types_checked() {
        let a = validate_alert(JobAlertInput {
            q: " ru%st_ ".into(),
            types: vec!["contract".into(), "contract".into()],
            ..Default::default()
        })
        .unwrap();
        assert_eq!(a.q, "rust");
        assert_eq!(a.types, vec!["contract".to_string()]);
        assert!(validate_alert(JobAlertInput {
            types: vec!["gig".into()],
            ..Default::default()
        })
        .is_err());
    }

    #[test]
    fn remote_only_is_a_valid_alert() {
        assert!(validate_alert(JobAlertInput {
            remote: true,
            ..Default::default()
        })
        .is_ok());
    }
}
