//! Teams/agencies + analytics_daily rollups (Phase 6).

#![forbid(unsafe_code)]

use chrono::{DateTime, NaiveDate, Utc};
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Team {
    pub id: Uuid,
    pub slug: String,
    pub name: String,
    pub summary: String,
    pub agency_profile: bool,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CreateTeamRequest {
    pub name: String,
    pub slug: String,
    #[serde(default)]
    pub summary: String,
    #[serde(default)]
    pub agency_profile: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricPoint {
    pub day: NaiveDate,
    pub metric: String,
    pub value: f64,
}

#[derive(Clone)]
pub struct AnalyticsService {
    pool: PgPool,
}

impl AnalyticsService {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn create_team(
        &self,
        account_id: Uuid,
        req: CreateTeamRequest,
    ) -> Result<Team, DomainError> {
        let name = req.name.trim();
        let slug = req.slug.trim().to_lowercase();
        if name.is_empty() || slug.is_empty() {
            return Err(DomainError::Validation("name and slug are required".into()));
        }
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO marketplace_teams (
              id, slug, name, summary, agency_profile, owner_account_id, created_at, updated_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$7)
            "#,
        )
        .bind(id)
        .bind(&slug)
        .bind(name)
        .bind(&req.summary)
        .bind(req.agency_profile)
        .bind(account_id)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(Team {
            id,
            slug,
            name: name.into(),
            summary: req.summary,
            agency_profile: req.agency_profile,
            created_at: now,
        })
    }

    pub async fn list_teams_for_owner(&self, account_id: Uuid) -> Result<Vec<Team>, DomainError> {
        let rows = sqlx::query_as::<_, (Uuid, String, String, String, bool, DateTime<Utc>)>(
            r#"
            SELECT id, slug, name, summary, agency_profile, created_at
            FROM marketplace_teams
            WHERE owner_account_id = $1
            ORDER BY created_at DESC
            "#,
        )
        .bind(account_id)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows
            .into_iter()
            .map(
                |(id, slug, name, summary, agency_profile, created_at)| Team {
                    id,
                    slug,
                    name,
                    summary,
                    agency_profile,
                    created_at,
                },
            )
            .collect())
    }

    pub async fn record_metric(
        &self,
        day: NaiveDate,
        subject_type: &str,
        subject_id: &str,
        metric: &str,
        value: f64,
    ) -> Result<(), DomainError> {
        sqlx::query(
            r#"
            INSERT INTO analytics_daily (day, subject_type, subject_id, metric, value)
            VALUES ($1,$2,$3,$4,$5)
            ON CONFLICT (day, subject_type, subject_id, metric)
            DO UPDATE SET value = EXCLUDED.value
            "#,
        )
        .bind(day)
        .bind(subject_type)
        .bind(subject_id)
        .bind(metric)
        .bind(value)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(())
    }

    pub async fn metrics_for(
        &self,
        subject_type: &str,
        subject_id: &str,
        limit: i64,
    ) -> Result<Vec<MetricPoint>, DomainError> {
        let limit = limit.clamp(1, 90);
        let rows = sqlx::query_as::<_, (NaiveDate, String, f64)>(
            r#"
            SELECT day, metric, value
            FROM analytics_daily
            WHERE subject_type = $1 AND subject_id = $2
            ORDER BY day DESC
            LIMIT $3
            "#,
        )
        .bind(subject_type)
        .bind(subject_id)
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows
            .into_iter()
            .map(|(day, metric, value)| MetricPoint { day, metric, value })
            .collect())
    }
}
