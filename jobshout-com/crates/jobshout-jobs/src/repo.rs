use chrono::{DateTime, Utc};
use jobshout_domain::{
    Compensation, CreateJobRequest, DomainError, EmploymentType, Job, JobId, JobStatus, Location,
    OrganisationId,
};
use sqlx::{PgPool, Row};
use uuid::Uuid;

#[derive(Clone)]
pub struct JobRepository {
    pool: PgPool,
}

impl JobRepository {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn list(
        &self,
        limit: i64,
        offset: i64,
        status: Option<JobStatus>,
    ) -> Result<Vec<Job>, DomainError> {
        let status_filter = status.map(|s| s.as_str());
        let rows = sqlx::query(
            r#"
            SELECT id, organisation_id, title, summary, description, employment_type,
                   location, compensation, requirements, status, created_at, updated_at, published_at
            FROM jobs
            WHERE ($1::text IS NULL OR status = $1)
            ORDER BY COALESCE(published_at, created_at) DESC
            LIMIT $2 OFFSET $3
            "#,
        )
        .bind(status_filter)
        .bind(limit)
        .bind(offset)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        rows.iter().map(job_from_row).collect()
    }

    pub async fn get(&self, id: JobId) -> Result<Job, DomainError> {
        let row = sqlx::query(
            r#"
            SELECT id, organisation_id, title, summary, description, employment_type,
                   location, compensation, requirements, status, created_at, updated_at, published_at
            FROM jobs
            WHERE id = $1
            "#,
        )
        .bind(id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        match row {
            Some(r) => job_from_row(&r),
            None => Err(DomainError::NotFound),
        }
    }

    pub async fn create(
        &self,
        org_id: OrganisationId,
        req: CreateJobRequest,
        status: JobStatus,
        poster_email: &str,
    ) -> Result<Job, DomainError> {
        let id = Uuid::new_v4();
        let now = Utc::now();
        let published_at = if status == JobStatus::Published {
            Some(now)
        } else {
            None
        };
        let location =
            serde_json::to_value(&req.location).map_err(|e| DomainError::Other(e.into()))?;
        let compensation =
            serde_json::to_value(&req.compensation).map_err(|e| DomainError::Other(e.into()))?;
        let requirements =
            serde_json::to_value(&req.requirements).map_err(|e| DomainError::Other(e.into()))?;

        sqlx::query(
            r#"
            INSERT INTO jobs (
              id, organisation_id, title, summary, description, employment_type,
              location, compensation, requirements, status, created_at, updated_at, published_at,
              poster_email
            ) VALUES (
              $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
            )
            "#,
        )
        .bind(id)
        .bind(org_id)
        .bind(req.title.trim())
        .bind(req.summary.trim())
        .bind(req.description.trim())
        .bind(req.employment_type.as_str())
        .bind(location)
        .bind(compensation)
        .bind(requirements)
        .bind(status.as_str())
        .bind(now)
        .bind(now)
        .bind(published_at)
        .bind(poster_email.trim().to_ascii_lowercase())
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        self.get(id).await
    }

    pub async fn poster_email(&self, id: JobId) -> Result<String, DomainError> {
        let row: Option<(String,)> = sqlx::query_as("SELECT poster_email FROM jobs WHERE id = $1")
            .bind(id)
            .fetch_optional(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        match row {
            Some((email,)) => Ok(email),
            None => Err(DomainError::NotFound),
        }
    }

    pub async fn update(&self, id: JobId, req: CreateJobRequest) -> Result<Job, DomainError> {
        let now = Utc::now();
        let location =
            serde_json::to_value(&req.location).map_err(|e| DomainError::Other(e.into()))?;
        let compensation =
            serde_json::to_value(&req.compensation).map_err(|e| DomainError::Other(e.into()))?;
        let requirements =
            serde_json::to_value(&req.requirements).map_err(|e| DomainError::Other(e.into()))?;
        // Publish when asked; otherwise leave status alone (drafts stay drafts).
        let publish = req.publish;
        let result = if publish {
            sqlx::query(
                r#"
                UPDATE jobs SET
                  title = $2, summary = $3, description = $4, employment_type = $5,
                  location = $6, compensation = $7, requirements = $8,
                  status = $9,
                  published_at = COALESCE(published_at, $10),
                  updated_at = $10
                WHERE id = $1
                "#,
            )
            .bind(id)
            .bind(req.title.trim())
            .bind(req.summary.trim())
            .bind(req.description.trim())
            .bind(req.employment_type.as_str())
            .bind(location)
            .bind(compensation)
            .bind(requirements)
            .bind(JobStatus::Published.as_str())
            .bind(now)
            .execute(&self.pool)
            .await
        } else {
            sqlx::query(
                r#"
                UPDATE jobs SET
                  title = $2, summary = $3, description = $4, employment_type = $5,
                  location = $6, compensation = $7, requirements = $8,
                  updated_at = $9
                WHERE id = $1
                "#,
            )
            .bind(id)
            .bind(req.title.trim())
            .bind(req.summary.trim())
            .bind(req.description.trim())
            .bind(req.employment_type.as_str())
            .bind(location)
            .bind(compensation)
            .bind(requirements)
            .bind(now)
            .execute(&self.pool)
            .await
        }
        .map_err(|e| DomainError::Other(e.into()))?;
        if result.rows_affected() == 0 {
            return Err(DomainError::NotFound);
        }
        self.get(id).await
    }

    pub async fn close(&self, id: JobId) -> Result<Job, DomainError> {
        let now = Utc::now();
        let result = sqlx::query(
            r#"
            UPDATE jobs SET status = 'closed', updated_at = $2
            WHERE id = $1 AND status <> 'closed'
            "#,
        )
        .bind(id)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        if result.rows_affected() == 0 {
            // Already closed or missing — distinguish.
            let _ = self.get(id).await?;
        }
        self.get(id).await
    }
}

/// Map one `jobs` row into a [`Job`]. Shared with other crates that join on jobs.
pub fn job_from_row(row: &sqlx::postgres::PgRow) -> Result<Job, DomainError> {
    let employment_type =
        EmploymentType::parse(row.get::<String, _>("employment_type").as_str())
            .ok_or_else(|| DomainError::Other(anyhow::anyhow!("invalid employment_type")))?;
    let status = JobStatus::parse(row.get::<String, _>("status").as_str())
        .ok_or_else(|| DomainError::Other(anyhow::anyhow!("invalid status")))?;
    let location: Location =
        serde_json::from_value(row.get("location")).map_err(|e| DomainError::Other(e.into()))?;
    let compensation: Compensation = serde_json::from_value(row.get("compensation"))
        .map_err(|e| DomainError::Other(e.into()))?;
    let requirements: Vec<String> = serde_json::from_value(row.get("requirements"))
        .map_err(|e| DomainError::Other(e.into()))?;

    Ok(Job {
        id: row.get("id"),
        organisation_id: row.get("organisation_id"),
        title: row.get("title"),
        summary: row.get("summary"),
        description: row.get("description"),
        employment_type,
        location,
        compensation,
        requirements,
        status,
        created_at: row.get::<DateTime<Utc>, _>("created_at"),
        updated_at: row.get::<DateTime<Utc>, _>("updated_at"),
        published_at: row.get("published_at"),
    })
}
