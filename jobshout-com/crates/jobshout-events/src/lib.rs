//! Domain event spine: append-only `domain_events` for feed / notifications.

#![forbid(unsafe_code)]

use chrono::{DateTime, Utc};
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sqlx::PgPool;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DomainEvent {
    pub id: Uuid,
    pub event_type: String,
    pub aggregate_type: String,
    pub aggregate_id: String,
    pub actor_account_id: Option<Uuid>,
    pub payload: Value,
    pub created_at: DateTime<Utc>,
}

#[derive(Clone)]
pub struct EventService {
    pool: PgPool,
}

impl EventService {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn emit(
        &self,
        event_type: &str,
        aggregate_type: &str,
        aggregate_id: &str,
        actor_account_id: Option<Uuid>,
        payload: Value,
    ) -> Result<DomainEvent, DomainError> {
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO domain_events (
              id, event_type, aggregate_type, aggregate_id, actor_account_id, payload, created_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7)
            "#,
        )
        .bind(id)
        .bind(event_type)
        .bind(aggregate_type)
        .bind(aggregate_id)
        .bind(actor_account_id)
        .bind(&payload)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(DomainEvent {
            id,
            event_type: event_type.into(),
            aggregate_type: aggregate_type.into(),
            aggregate_id: aggregate_id.into(),
            actor_account_id,
            payload,
            created_at: now,
        })
    }
}
