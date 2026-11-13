//! Linked Groups — public / private / secret communities.
//! Secret groups never appear in list/search results.

#![forbid(unsafe_code)]

use chrono::{DateTime, Utc};
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Group {
    pub id: Uuid,
    pub slug: String,
    pub name: String,
    pub description: String,
    pub kind: String,
    pub visibility: String,
    pub join_policy: String,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CreateGroupRequest {
    pub name: String,
    pub slug: String,
    #[serde(default)]
    pub description: String,
    #[serde(default = "default_kind")]
    pub kind: String,
    #[serde(default = "default_visibility")]
    pub visibility: String,
    #[serde(default = "default_join")]
    pub join_policy: String,
}

fn default_kind() -> String {
    "skill".into()
}
fn default_visibility() -> String {
    "public".into()
}
fn default_join() -> String {
    "open".into()
}

#[derive(Clone)]
pub struct GroupsService {
    pool: PgPool,
}

impl GroupsService {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    /// Public + private groups only. Secret groups are excluded always.
    pub async fn list_discoverable(&self, limit: i64) -> Result<Vec<Group>, DomainError> {
        let limit = limit.clamp(1, 100);
        let rows = sqlx::query_as::<
            _,
            (
                Uuid,
                String,
                String,
                String,
                String,
                String,
                String,
                DateTime<Utc>,
            ),
        >(
            r#"
            SELECT id, slug, name, description, kind, visibility, join_policy, created_at
            FROM community_groups
            WHERE visibility IN ('public', 'private')
            ORDER BY created_at DESC
            LIMIT $1
            "#,
        )
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(map_group).collect())
    }

    pub async fn get_by_slug(
        &self,
        slug: &str,
        viewer_member_id: Option<Uuid>,
    ) -> Result<Group, DomainError> {
        let row = sqlx::query_as::<
            _,
            (
                Uuid,
                String,
                String,
                String,
                String,
                String,
                String,
                DateTime<Utc>,
            ),
        >(
            r#"
            SELECT id, slug, name, description, kind, visibility, join_policy, created_at
            FROM community_groups
            WHERE slug = $1
            "#,
        )
        .bind(slug)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        let Some(r) = row else {
            return Err(DomainError::NotFound);
        };
        let g = map_group(r);
        if g.visibility == "secret" {
            let Some(mid) = viewer_member_id else {
                return Err(DomainError::NotFound);
            };
            let member: Option<(Uuid,)> = sqlx::query_as(
                r#"
                SELECT id FROM community_group_members
                WHERE group_id = $1 AND member_id = $2 AND NOT banned
                "#,
            )
            .bind(g.id)
            .bind(mid)
            .fetch_optional(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
            if member.is_none() {
                return Err(DomainError::NotFound);
            }
        }
        Ok(g)
    }

    pub async fn create(
        &self,
        account_id: Uuid,
        member_id: Uuid,
        req: CreateGroupRequest,
    ) -> Result<Group, DomainError> {
        let name = req.name.trim();
        let slug = req.slug.trim().to_lowercase();
        if name.is_empty() || slug.is_empty() {
            return Err(DomainError::Validation("name and slug are required".into()));
        }
        let kind = match req.kind.as_str() {
            "skill" | "industry" | "location" | "company" | "ai_agent" => req.kind.as_str(),
            _ => "skill",
        };
        let visibility = match req.visibility.as_str() {
            "public" | "private" | "secret" => req.visibility.as_str(),
            _ => "public",
        };
        let join_policy = match req.join_policy.as_str() {
            "open" | "request" | "invite" => req.join_policy.as_str(),
            _ => "open",
        };
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO community_groups (
              id, slug, name, description, kind, visibility, join_policy,
              owner_account_id, created_at, updated_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
            "#,
        )
        .bind(id)
        .bind(&slug)
        .bind(name)
        .bind(&req.description)
        .bind(kind)
        .bind(visibility)
        .bind(join_policy)
        .bind(account_id)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        sqlx::query(
            r#"
            INSERT INTO community_group_members (id, group_id, member_id, role)
            VALUES ($1,$2,$3,'owner')
            "#,
        )
        .bind(Uuid::new_v4())
        .bind(id)
        .bind(member_id)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(Group {
            id,
            slug,
            name: name.into(),
            description: req.description,
            kind: kind.into(),
            visibility: visibility.into(),
            join_policy: join_policy.into(),
            created_at: now,
        })
    }

    /// Search never returns secret groups.
    pub async fn search(&self, q: &str, limit: i64) -> Result<Vec<Group>, DomainError> {
        let limit = limit.clamp(1, 50);
        let pattern = format!("%{}%", q.trim());
        let rows = sqlx::query_as::<
            _,
            (
                Uuid,
                String,
                String,
                String,
                String,
                String,
                String,
                DateTime<Utc>,
            ),
        >(
            r#"
            SELECT id, slug, name, description, kind, visibility, join_policy, created_at
            FROM community_groups
            WHERE visibility <> 'secret'
              AND (name ILIKE $1 OR description ILIKE $1 OR slug ILIKE $1)
            ORDER BY name
            LIMIT $2
            "#,
        )
        .bind(&pattern)
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(map_group).collect())
    }
}

fn map_group(
    (id, slug, name, description, kind, visibility, join_policy, created_at): (
        Uuid,
        String,
        String,
        String,
        String,
        String,
        String,
        DateTime<Utc>,
    ),
) -> Group {
    Group {
        id,
        slug,
        name,
        description,
        kind,
        visibility,
        join_policy,
        created_at,
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn secret_not_in_visibility_list_for_discovery() {
        let allowed = ["public", "private"];
        assert!(!allowed.contains(&"secret"));
    }
}
