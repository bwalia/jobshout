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

        rows.into_iter().map(map_row).collect()
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
            Some(r) => map_row(r),
            None => Err(DomainError::NotFound),
        }
    }

    pub async fn create(
        &self,
        org_id: OrganisationId,
        req: CreateJobRequest,
        status: JobStatus,
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
              location, compensation, requirements, status, created_at, updated_at, published_at
            ) VALUES (
              $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
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
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        self.get(id).await
    }
}

fn map_row(row: sqlx::postgres::PgRow) -> Result<Job, DomainError> {
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
