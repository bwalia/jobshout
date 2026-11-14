use chrono::{DateTime, Utc};
use jobshout_domain::{
    ApplicationStatus, CreateJobApplicationRequest, DomainError, JobApplication, JobApplicationId,
    JobApplicationWithJob, JobId,
};
use sqlx::{PgPool, Row};
use uuid::Uuid;

/// Application columns, aliased so they never collide with the joined job's
/// `id` / `status` / `created_at` / `updated_at`.
const APP_COLUMNS: &str = r#"
    a.id            AS app_id,
    a.job_id        AS app_job_id,
    a.candidate_profile_id AS app_candidate_profile_id,
    a.full_name     AS app_full_name,
    a.email         AS app_email,
    a.phone         AS app_phone,
    a.cover_letter  AS app_cover_letter,
    a.cv_text       AS app_cv_text,
    a.cv_url        AS app_cv_url,
    a.status        AS app_status,
    a.created_at    AS app_created_at,
    a.updated_at    AS app_updated_at
"#;

#[derive(Clone)]
pub struct ApplicationRepository {
    pool: PgPool,
}

impl ApplicationRepository {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    /// Insert the application, or update it if this email already applied to this job.
    /// Links to a candidate profile when one exists for the same email.
    pub async fn upsert(
        &self,
        job_id: JobId,
        req: CreateJobApplicationRequest,
    ) -> Result<JobApplication, DomainError> {
        // Only live jobs take applications. Drafts and closed roles answer
        // NotFound, the same as a missing job, so their details never leak.
        let job_status: Option<(String,)> = sqlx::query_as("SELECT status FROM jobs WHERE id = $1")
            .bind(job_id)
            .fetch_optional(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        match job_status {
            None => return Err(DomainError::NotFound),
            Some((s,)) if s == "published" => {}
            Some((s,)) if s == "closed" || s == "archived" => {
                return Err(DomainError::Validation(
                    "this job is closed and no longer takes applications".into(),
                ))
            }
            Some(_) => return Err(DomainError::NotFound),
        }

        // Once the hiring team has moved an application on, the candidate
        // cannot silently rewrite it. A withdrawn one may be sent again.
        if let Some(existing) = self.find(job_id, &req.email).await? {
            if !matches!(
                existing.status,
                ApplicationStatus::Submitted | ApplicationStatus::Withdrawn
            ) {
                return Err(DomainError::Conflict(format!(
                    "your application is already {} and can no longer be changed",
                    existing.status.as_str()
                )));
            }
        }

        let sql = format!(
            r#"
            INSERT INTO job_applications (
              id, job_id, candidate_profile_id, full_name, email, phone,
              cover_letter, cv_text, cv_url, status, created_at, updated_at
            )
            VALUES (
              $1, $2,
              (SELECT p.id FROM candidate_profiles p WHERE lower(p.email) = lower($4) LIMIT 1),
              $3, $4, $5, $6, $7, $8, 'submitted', NOW(), NOW()
            )
            ON CONFLICT (job_id, lower(email)) DO UPDATE SET
              candidate_profile_id = COALESCE(EXCLUDED.candidate_profile_id, job_applications.candidate_profile_id),
              full_name    = EXCLUDED.full_name,
              -- A re-send that leaves a field blank keeps what was there.
              phone        = COALESCE(NULLIF(EXCLUDED.phone, ''), job_applications.phone),
              cover_letter = COALESCE(NULLIF(EXCLUDED.cover_letter, ''), job_applications.cover_letter),
              cv_text      = COALESCE(NULLIF(EXCLUDED.cv_text, ''), job_applications.cv_text),
              cv_url       = COALESCE(NULLIF(EXCLUDED.cv_url, ''), job_applications.cv_url),
              status       = 'submitted',
              updated_at   = NOW()
            RETURNING {cols}
            "#,
            cols = APP_COLUMNS.replace("a.", ""),
        );

        let row = sqlx::query(&sql)
            .bind(Uuid::new_v4())
            .bind(job_id)
            .bind(&req.full_name)
            .bind(&req.email)
            .bind(&req.phone)
            .bind(&req.cover_letter)
            .bind(&req.cv_text)
            .bind(&req.cv_url)
            .fetch_one(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;

        application_from_row(&row)
    }

    pub async fn find(
        &self,
        job_id: JobId,
        email: &str,
    ) -> Result<Option<JobApplication>, DomainError> {
        let sql = format!(
            "SELECT {APP_COLUMNS} FROM job_applications a \
             WHERE a.job_id = $1 AND lower(a.email) = lower($2)"
        );
        let row = sqlx::query(&sql)
            .bind(job_id)
            .bind(email)
            .fetch_optional(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        row.as_ref().map(application_from_row).transpose()
    }

    pub async fn list_for_job(
        &self,
        job_id: JobId,
        limit: i64,
        offset: i64,
    ) -> Result<Vec<JobApplication>, DomainError> {
        let sql = format!(
            "SELECT {APP_COLUMNS} FROM job_applications a \
             WHERE a.job_id = $1 ORDER BY a.created_at DESC LIMIT $2 OFFSET $3"
        );
        let rows = sqlx::query(&sql)
            .bind(job_id)
            .bind(limit)
            .bind(offset)
            .fetch_all(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        rows.iter().map(application_from_row).collect()
    }

    pub async fn list_for_email(
        &self,
        email: &str,
        limit: i64,
    ) -> Result<Vec<JobApplicationWithJob>, DomainError> {
        let sql = format!(
            r#"
            SELECT {APP_COLUMNS},
                   j.id, j.organisation_id, j.title, j.summary, j.description,
                   j.employment_type, j.location, j.compensation, j.requirements,
                   j.status, j.created_at, j.updated_at, j.published_at
            FROM job_applications a
            JOIN jobs j ON j.id = a.job_id
            WHERE lower(a.email) = lower($1)
            ORDER BY a.created_at DESC
            LIMIT $2
            "#
        );
        let rows = sqlx::query(&sql)
            .bind(email)
            .bind(limit)
            .fetch_all(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;

        rows.iter()
            .map(|r| {
                Ok(JobApplicationWithJob {
                    application: application_from_row(r)?,
                    job: jobshout_jobs::job_from_row(r)?,
                })
            })
            .collect()
    }

    pub async fn set_status(
        &self,
        job_id: JobId,
        application_id: JobApplicationId,
        status: ApplicationStatus,
    ) -> Result<JobApplication, DomainError> {
        let sql = format!(
            r#"
            UPDATE job_applications SET status = $3, updated_at = NOW()
            WHERE id = $1 AND job_id = $2 AND status <> 'withdrawn'
            RETURNING {cols}
            "#,
            cols = APP_COLUMNS.replace("a.", ""),
        );
        let row = sqlx::query(&sql)
            .bind(application_id)
            .bind(job_id)
            .bind(status.as_str())
            .fetch_optional(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        match row {
            Some(row) => application_from_row(&row),
            None => {
                let exists: Option<(String,)> = sqlx::query_as(
                    "SELECT status FROM job_applications WHERE id = $1 AND job_id = $2",
                )
                .bind(application_id)
                .bind(job_id)
                .fetch_optional(&self.pool)
                .await
                .map_err(|e| DomainError::Other(e.into()))?;
                Err(match exists {
                    Some(_) => {
                        DomainError::Conflict("the candidate withdrew this application".into())
                    }
                    None => DomainError::NotFound,
                })
            }
        }
    }

    pub async fn withdraw(
        &self,
        application_id: JobApplicationId,
        email: &str,
    ) -> Result<JobApplication, DomainError> {
        let sql = format!(
            r#"
            UPDATE job_applications SET status = 'withdrawn', updated_at = NOW()
            WHERE id = $1 AND lower(email) = lower($2)
            RETURNING {cols}
            "#,
            cols = APP_COLUMNS.replace("a.", ""),
        );
        let row = sqlx::query(&sql)
            .bind(application_id)
            .bind(email)
            .fetch_optional(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        // Someone else's application looks exactly like a missing one.
        row.as_ref()
            .map(application_from_row)
            .transpose()?
            .ok_or(DomainError::NotFound)
    }

    pub async fn count_for_job(&self, job_id: JobId) -> Result<i64, DomainError> {
        let row: (i64,) = sqlx::query_as("SELECT COUNT(*) FROM job_applications WHERE job_id = $1")
            .bind(job_id)
            .fetch_one(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        Ok(row.0)
    }
}

fn application_from_row(row: &sqlx::postgres::PgRow) -> Result<JobApplication, DomainError> {
    let status = ApplicationStatus::parse(row.get::<String, _>("app_status").as_str())
        .ok_or_else(|| DomainError::Other(anyhow::anyhow!("invalid application status")))?;

    Ok(JobApplication {
        id: row.get("app_id"),
        job_id: row.get("app_job_id"),
        candidate_profile_id: row.get("app_candidate_profile_id"),
        full_name: row.get("app_full_name"),
        email: row.get("app_email"),
        phone: row.get("app_phone"),
        cover_letter: row.get("app_cover_letter"),
        cv_text: row.get("app_cv_text"),
        cv_url: row.get("app_cv_url"),
        status,
        created_at: row.get::<DateTime<Utc>, _>("app_created_at"),
        updated_at: row.get::<DateTime<Utc>, _>("app_updated_at"),
    })
}
