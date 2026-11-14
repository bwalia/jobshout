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

/// Notification kinds the platform sends, with the label shown in settings.
pub const KINDS: &[(&str, &str)] = &[
    (
        "application_received",
        "Someone applies to a job you posted",
    ),
    (
        "application_status",
        "Your application moves to a new stage",
    ),
    ("message", "New direct messages"),
    ("connection_request", "Connection requests"),
    ("connection_accepted", "Accepted connection requests"),
    ("job_alert", "New jobs matching your job alerts"),
];

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Preference {
    pub kind: String,
    pub label: String,
    pub in_app: bool,
}

#[derive(sqlx::FromRow)]
struct NotificationRow {
    id: Uuid,
    account_id: Uuid,
    kind: String,
    title: String,
    body: String,
    href: String,
    payload: Value,
    read_at: Option<DateTime<Utc>>,
    created_at: DateTime<Utc>,
}

impl From<NotificationRow> for Notification {
    fn from(r: NotificationRow) -> Self {
        Self {
            id: r.id,
            account_id: r.account_id,
            kind: r.kind,
            title: r.title,
            body: r.body,
            href: r.href,
            payload: r.payload,
            read_at: r.read_at,
            created_at: r.created_at,
        }
    }
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

    /// Notify the account behind `email`, if one exists and has not turned
    /// this kind of in-app notification off. Never fails the caller: a
    /// notification is a side effect, so problems are logged.
    pub async fn notify_email(
        &self,
        email: &str,
        kind: &str,
        title: &str,
        body: &str,
        href: &str,
        payload: Value,
    ) {
        let result: Result<(), DomainError> = async {
            let Some(account_id) = self.account_for_email(email).await? else {
                return Ok(());
            };
            if !self.enabled(account_id, "in_app", kind).await? {
                return Ok(());
            }
            self.notify(account_id, kind, title, body, href, payload)
                .await
                .map(|_| ())
        }
        .await;
        if let Err(e) = result {
            tracing::warn!(error = ?e, kind, "could not create notification");
        }
    }

    async fn account_for_email(&self, email: &str) -> Result<Option<Uuid>, DomainError> {
        sqlx::query_scalar("SELECT id FROM accounts WHERE lower(primary_email) = lower($1)")
            .bind(email.trim())
            .fetch_optional(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))
    }

    /// Preferences default to on; a row only exists once someone turns one off.
    pub async fn enabled(
        &self,
        account_id: Uuid,
        channel: &str,
        kind: &str,
    ) -> Result<bool, DomainError> {
        let v: Option<bool> = sqlx::query_scalar(
            "SELECT enabled FROM notification_preferences \
             WHERE account_id = $1 AND channel = $2 AND kind = $3",
        )
        .bind(account_id)
        .bind(channel)
        .bind(kind)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(v.unwrap_or(true))
    }

    /// The account's own notifications, newest first.
    pub async fn list(
        &self,
        account_id: Uuid,
        limit: i64,
        unread_only: bool,
    ) -> Result<Vec<Notification>, DomainError> {
        let rows = sqlx::query_as::<_, NotificationRow>(
            r#"
            SELECT id, account_id, kind, title, body, href, payload, read_at, created_at
            FROM notifications
            WHERE account_id = $1 AND (NOT $3 OR read_at IS NULL)
            ORDER BY created_at DESC
            LIMIT $2
            "#,
        )
        .bind(account_id)
        .bind(limit.clamp(1, 100))
        .bind(unread_only)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(Into::into).collect())
    }

    pub async fn unread_count(&self, account_id: Uuid) -> Result<i64, DomainError> {
        sqlx::query_scalar(
            "SELECT COUNT(*) FROM notifications WHERE account_id = $1 AND read_at IS NULL",
        )
        .bind(account_id)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))
    }

    /// Mark the given notifications (or all, when `ids` is empty) read.
    /// Only the account's own rows are touched.
    pub async fn mark_read(&self, account_id: Uuid, ids: &[Uuid]) -> Result<u64, DomainError> {
        let r = sqlx::query(
            r#"
            UPDATE notifications SET read_at = NOW()
            WHERE account_id = $1 AND read_at IS NULL
              AND (cardinality($2::uuid[]) = 0 OR id = ANY($2))
            "#,
        )
        .bind(account_id)
        .bind(ids)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(r.rows_affected())
    }

    /// Every known kind with its in-app setting for this account.
    pub async fn preferences(&self, account_id: Uuid) -> Result<Vec<Preference>, DomainError> {
        let mut out = Vec::new();
        for (kind, label) in KINDS {
            out.push(Preference {
                kind: (*kind).into(),
                label: (*label).into(),
                in_app: self.enabled(account_id, "in_app", kind).await?,
            });
        }
        Ok(out)
    }

    pub async fn set_preference(
        &self,
        account_id: Uuid,
        kind: &str,
        in_app: bool,
    ) -> Result<(), DomainError> {
        if !KINDS.iter().any(|(k, _)| *k == kind) {
            return Err(DomainError::Validation(format!(
                "unknown notification kind: {kind}"
            )));
        }
        sqlx::query(
            r#"
            INSERT INTO notification_preferences (account_id, channel, kind, enabled)
            VALUES ($1, 'in_app', $2, $3)
            ON CONFLICT (account_id, channel, kind) DO UPDATE SET enabled = EXCLUDED.enabled
            "#,
        )
        .bind(account_id)
        .bind(kind)
        .bind(in_app)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(())
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
