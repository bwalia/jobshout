//! Conversations and messages (Phase 1).
//!
//! Permission rules: a member may only list conversations they participate in,
//! and may only send to those. Direct conversations are 1:1; block lists stop
//! new threads. Gated by `MESSAGING_ENABLED` at the HTTP layer.

#![forbid(unsafe_code)]

use chrono::{DateTime, Utc};
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use sqlx::{PgPool, Row};
use uuid::Uuid;

fn db(e: sqlx::Error) -> DomainError {
    DomainError::Other(e.into())
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConversationSummary {
    pub id: Uuid,
    pub kind: String,
    pub subject: String,
    pub updated_at: DateTime<Utc>,
    pub unread: i64,
    pub peer_handle: Option<String>,
    pub peer_display_name: Option<String>,
    pub last_body: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub id: Uuid,
    pub conversation_id: Uuid,
    pub sender_id: Uuid,
    pub sender_handle: String,
    pub body: String,
    pub created_at: DateTime<Utc>,
}

#[derive(Clone)]
pub struct MessagingService {
    pool: PgPool,
}

impl MessagingService {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn list_conversations(
        &self,
        member_id: Uuid,
        limit: i64,
    ) -> Result<Vec<ConversationSummary>, DomainError> {
        let limit = limit.clamp(1, 50);
        let rows = sqlx::query(
            r#"
            SELECT c.id, c.kind, c.subject, c.updated_at,
                   (SELECT COUNT(*) FROM messages m
                      WHERE m.conversation_id = c.id
                        AND m.sender_id <> $1
                        AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
                   ) AS unread,
                   peer.handle AS peer_handle,
                   peer.display_name AS peer_display_name,
                   (SELECT body FROM messages m
                      WHERE m.conversation_id = c.id
                      ORDER BY m.created_at DESC LIMIT 1) AS last_body
            FROM conversation_participants p
            JOIN conversations c ON c.id = p.conversation_id
            LEFT JOIN LATERAL (
              SELECT m.handle, m.display_name
              FROM conversation_participants cp
              JOIN members m ON m.id = cp.member_id
              WHERE cp.conversation_id = c.id AND cp.member_id <> $1
              LIMIT 1
            ) peer ON TRUE
            WHERE p.member_id = $1
            ORDER BY c.updated_at DESC
            LIMIT $2
            "#,
        )
        .bind(member_id)
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| ConversationSummary {
                id: r.get("id"),
                kind: r.get("kind"),
                subject: r.get("subject"),
                updated_at: r.get("updated_at"),
                unread: r.get("unread"),
                peer_handle: r.get("peer_handle"),
                peer_display_name: r.get("peer_display_name"),
                last_body: r.get("last_body"),
            })
            .collect())
    }

    pub async fn open_direct(
        &self,
        me: Uuid,
        peer: Uuid,
    ) -> Result<ConversationSummary, DomainError> {
        if me == peer {
            return Err(DomainError::Validation("cannot message yourself".into()));
        }
        let blocked: bool = sqlx::query_scalar(
            r#"
            SELECT EXISTS (
              SELECT 1 FROM member_blocks
              WHERE (blocker_id = $1 AND blocked_id = $2)
                 OR (blocker_id = $2 AND blocked_id = $1)
            )
            "#,
        )
        .bind(me)
        .bind(peer)
        .fetch_one(&self.pool)
        .await
        .map_err(db)?;
        if blocked {
            return Err(DomainError::Forbidden("messaging blocked".into()));
        }

        // Reuse an existing 1:1 direct conversation if present.
        if let Some(existing) = sqlx::query_scalar::<_, Uuid>(
            r#"
            SELECT c.id FROM conversations c
            JOIN conversation_participants a ON a.conversation_id = c.id AND a.member_id = $1
            JOIN conversation_participants b ON b.conversation_id = c.id AND b.member_id = $2
            WHERE c.kind = 'direct'
              AND (SELECT COUNT(*) FROM conversation_participants p WHERE p.conversation_id = c.id) = 2
            LIMIT 1
            "#,
        )
        .bind(me)
        .bind(peer)
        .fetch_optional(&self.pool)
        .await
        .map_err(db)?
        {
            return self
                .list_conversations(me, 50)
                .await?
                .into_iter()
                .find(|c| c.id == existing)
                .ok_or(DomainError::NotFound);
        }

        let mut tx = self.pool.begin().await.map_err(db)?;
        let id: Uuid = sqlx::query_scalar(
            r#"
            INSERT INTO conversations (kind, subject, created_by)
            VALUES ('direct', '', $1)
            RETURNING id
            "#,
        )
        .bind(me)
        .fetch_one(&mut *tx)
        .await
        .map_err(db)?;
        for mid in [me, peer] {
            sqlx::query(
                r#"
                INSERT INTO conversation_participants (conversation_id, member_id)
                VALUES ($1, $2)
                "#,
            )
            .bind(id)
            .bind(mid)
            .execute(&mut *tx)
            .await
            .map_err(db)?;
        }
        tx.commit().await.map_err(db)?;
        self.list_conversations(me, 50)
            .await?
            .into_iter()
            .find(|c| c.id == id)
            .ok_or(DomainError::NotFound)
    }

    pub async fn list_messages(
        &self,
        member_id: Uuid,
        conversation_id: Uuid,
        limit: i64,
    ) -> Result<Vec<Message>, DomainError> {
        self.require_participant(member_id, conversation_id).await?;
        let limit = limit.clamp(1, 100);
        let rows = sqlx::query(
            r#"
            SELECT m.id, m.conversation_id, m.sender_id, m.body, m.created_at,
                   s.handle AS sender_handle
            FROM messages m
            JOIN members s ON s.id = m.sender_id
            WHERE m.conversation_id = $1
            ORDER BY m.created_at DESC
            LIMIT $2
            "#,
        )
        .bind(conversation_id)
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        // Mark read.
        sqlx::query(
            r#"
            UPDATE conversation_participants
            SET last_read_at = NOW()
            WHERE conversation_id = $1 AND member_id = $2
            "#,
        )
        .bind(conversation_id)
        .bind(member_id)
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| Message {
                id: r.get("id"),
                conversation_id: r.get("conversation_id"),
                sender_id: r.get("sender_id"),
                sender_handle: r.get("sender_handle"),
                body: r.get("body"),
                created_at: r.get("created_at"),
            })
            .collect())
    }

    pub async fn send(
        &self,
        member_id: Uuid,
        conversation_id: Uuid,
        body: &str,
    ) -> Result<Message, DomainError> {
        let body = body.trim();
        if body.is_empty() {
            return Err(DomainError::Validation("message body required".into()));
        }
        if body.chars().count() > 8000 {
            return Err(DomainError::Validation("message too long".into()));
        }
        self.require_participant(member_id, conversation_id).await?;
        let mut tx = self.pool.begin().await.map_err(db)?;
        let row = sqlx::query(
            r#"
            INSERT INTO messages (conversation_id, sender_id, body)
            VALUES ($1, $2, $3)
            RETURNING id, conversation_id, sender_id, body, created_at
            "#,
        )
        .bind(conversation_id)
        .bind(member_id)
        .bind(body)
        .fetch_one(&mut *tx)
        .await
        .map_err(db)?;
        sqlx::query("UPDATE conversations SET updated_at = NOW() WHERE id = $1")
            .bind(conversation_id)
            .execute(&mut *tx)
            .await
            .map_err(db)?;
        tx.commit().await.map_err(db)?;
        let handle: String = sqlx::query_scalar("SELECT handle FROM members WHERE id = $1")
            .bind(member_id)
            .fetch_one(&self.pool)
            .await
            .map_err(db)?;
        Ok(Message {
            id: row.get("id"),
            conversation_id: row.get("conversation_id"),
            sender_id: row.get("sender_id"),
            sender_handle: handle,
            body: row.get("body"),
            created_at: row.get("created_at"),
        })
    }

    pub async fn block(&self, blocker: Uuid, blocked: Uuid) -> Result<(), DomainError> {
        if blocker == blocked {
            return Err(DomainError::Validation("cannot block yourself".into()));
        }
        sqlx::query(
            r#"
            INSERT INTO member_blocks (blocker_id, blocked_id)
            VALUES ($1, $2)
            ON CONFLICT DO NOTHING
            "#,
        )
        .bind(blocker)
        .bind(blocked)
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(())
    }

    pub async fn report(
        &self,
        reporter: Uuid,
        subject: Uuid,
        reason: &str,
        detail: &str,
    ) -> Result<Uuid, DomainError> {
        let reason = reason.trim();
        if reason.is_empty() {
            return Err(DomainError::Validation("reason required".into()));
        }
        let id: Uuid = sqlx::query_scalar(
            r#"
            INSERT INTO member_reports (reporter_id, subject_id, reason, detail)
            VALUES ($1, $2, $3, $4)
            RETURNING id
            "#,
        )
        .bind(reporter)
        .bind(subject)
        .bind(reason.chars().take(200).collect::<String>())
        .bind(detail.chars().take(2000).collect::<String>())
        .fetch_one(&self.pool)
        .await
        .map_err(db)?;
        Ok(id)
    }

    pub async fn unread_count(&self, member_id: Uuid) -> Result<i64, DomainError> {
        sqlx::query_scalar(
            r#"
            SELECT COUNT(*) FROM messages m
            JOIN conversation_participants p ON p.conversation_id = m.conversation_id
            WHERE p.member_id = $1
              AND m.sender_id <> $1
              AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
            "#,
        )
        .bind(member_id)
        .fetch_one(&self.pool)
        .await
        .map_err(db)
    }

    async fn require_participant(
        &self,
        member_id: Uuid,
        conversation_id: Uuid,
    ) -> Result<(), DomainError> {
        let ok: bool = sqlx::query_scalar(
            r#"
            SELECT EXISTS (
              SELECT 1 FROM conversation_participants
              WHERE conversation_id = $1 AND member_id = $2
            )
            "#,
        )
        .bind(conversation_id)
        .bind(member_id)
        .fetch_one(&self.pool)
        .await
        .map_err(db)?;
        if ok {
            Ok(())
        } else {
            Err(DomainError::Forbidden(
                "not a participant in this conversation".into(),
            ))
        }
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn service_new_is_clone() {
        // MessagingService is Clone; this keeps the crate in the test graph.
        fn assert_clone<T: Clone>() {}
        assert_clone::<super::MessagingService>();
    }
}
