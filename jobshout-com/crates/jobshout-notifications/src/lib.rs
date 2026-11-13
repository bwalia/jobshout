//! In-app notifications and the email outbox (worker drains the outbox).

#![forbid(unsafe_code)]

use chrono::{DateTime, Duration, Utc};
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sqlx::PgPool;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Notification {
    pub id: Uuid,
    pub account_id: Uuid,
    pub kind: String,
    pub title: String,
    pub body: String,
    pub href: String,
    pub payload: Value,
    pub read_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OutboxEmail {
    pub id: Uuid,
    pub to_email: String,
    pub subject: String,
    pub body_text: String,
    pub body_html: String,
    pub status: String,
    pub attempts: i32,
    pub max_attempts: i32,
    pub last_error: String,
    pub scheduled_at: DateTime<Utc>,
    pub sent_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
}

#[derive(Clone)]
pub struct NotificationService {
    pool: PgPool,
}

impl NotificationService {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn notify(
        &self,
        account_id: Uuid,
        kind: &str,
        title: &str,
        body: &str,
        href: &str,
        payload: Value,
    ) -> Result<Notification, DomainError> {
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO notifications (
              id, account_id, kind, title, body, href, payload, created_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
            "#,
        )
        .bind(id)
        .bind(account_id)
        .bind(kind)
        .bind(title)
        .bind(body)
        .bind(href)
        .bind(&payload)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(Notification {
            id,
            account_id,
            kind: kind.into(),
            title: title.into(),
            body: body.into(),
            href: href.into(),
            payload,
            read_at: None,
            created_at: now,
        })
    }

    /// Queue an email for the worker. Feature flag: callers should check
    /// `EMAIL_OUTBOX_ENABLED` before relying on delivery.
    pub async fn enqueue_email(
        &self,
        to_email: &str,
        subject: &str,
        body_text: &str,
        body_html: &str,
    ) -> Result<OutboxEmail, DomainError> {
        let id = Uuid::new_v4();
        let now = Utc::now();
        let to = to_email.trim().to_ascii_lowercase();
        sqlx::query(
            r#"
            INSERT INTO email_outbox (
              id, to_email, subject, body_text, body_html, status, attempts, max_attempts,
              last_error, scheduled_at, created_at, updated_at
            ) VALUES ($1,$2,$3,$4,$5,'pending',0,8,'',$6,$6,$6)
            "#,
        )
        .bind(id)
        .bind(&to)
        .bind(subject)
        .bind(body_text)
        .bind(body_html)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(OutboxEmail {
            id,
            to_email: to,
            subject: subject.into(),
            body_text: body_text.into(),
            body_html: body_html.into(),
            status: "pending".into(),
            attempts: 0,
            max_attempts: 8,
            last_error: String::new(),
            scheduled_at: now,
            sent_at: None,
            created_at: now,
        })
    }

    /// Claim a batch of due outbox rows for sending.
    pub async fn claim_outbox(&self, limit: i64) -> Result<Vec<OutboxEmail>, DomainError> {
        let rows = sqlx::query_as::<_, OutboxRow>(
            r#"
            UPDATE email_outbox
            SET status = 'sending', updated_at = NOW(), attempts = attempts + 1
            WHERE id IN (
              SELECT id FROM email_outbox
              WHERE status IN ('pending', 'retry')
                AND scheduled_at <= NOW()
                AND attempts < max_attempts
              ORDER BY scheduled_at
              FOR UPDATE SKIP LOCKED
              LIMIT $1
            )
            RETURNING id, to_email, subject, body_text, body_html, status, attempts,
                      max_attempts, last_error, scheduled_at, sent_at, created_at
            "#,
        )
        .bind(limit.clamp(1, 100))
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(Into::into).collect())
    }

    pub async fn mark_sent(&self, id: Uuid) -> Result<(), DomainError> {
        sqlx::query(
            "UPDATE email_outbox SET status = 'sent', sent_at = NOW(), updated_at = NOW(), last_error = '' WHERE id = $1",
        )
        .bind(id)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(())
    }

    pub async fn mark_failed(&self, id: Uuid, error: &str) -> Result<(), DomainError> {
        let row: (i32, i32) =
            sqlx::query_as("SELECT attempts, max_attempts FROM email_outbox WHERE id = $1")
                .bind(id)
                .fetch_one(&self.pool)
                .await
                .map_err(|e| DomainError::Other(e.into()))?;
        let (attempts, max_attempts) = row;
        let status = if attempts >= max_attempts {
            "failed"
        } else {
            "retry"
        };
        // Exponential-ish backoff: 1m, 2m, 4m… capped at 1h.
        let delay_secs = (60i64 * (1 << (attempts.saturating_sub(1).min(6)))).min(3600);
        let when = Utc::now() + Duration::seconds(delay_secs);
        sqlx::query(
            r#"
            UPDATE email_outbox
            SET status = $2, last_error = $3, scheduled_at = $4, updated_at = NOW()
            WHERE id = $1
            "#,
        )
        .bind(id)
        .bind(status)
        .bind(error.chars().take(2000).collect::<String>())
        .bind(when)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(())
    }
}

#[derive(sqlx::FromRow)]
struct OutboxRow {
    id: Uuid,
    to_email: String,
    subject: String,
    body_text: String,
    body_html: String,
    status: String,
    attempts: i32,
    max_attempts: i32,
    last_error: String,
    scheduled_at: DateTime<Utc>,
    sent_at: Option<DateTime<Utc>>,
    created_at: DateTime<Utc>,
}

impl From<OutboxRow> for OutboxEmail {
    fn from(r: OutboxRow) -> Self {
        Self {
            id: r.id,
            to_email: r.to_email,
            subject: r.subject,
            body_text: r.body_text,
            body_html: r.body_html,
            status: r.status,
            attempts: r.attempts,
            max_attempts: r.max_attempts,
            last_error: r.last_error,
            scheduled_at: r.scheduled_at,
            sent_at: r.sent_at,
            created_at: r.created_at,
        }
    }
}
