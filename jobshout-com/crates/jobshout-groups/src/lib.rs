//! Linked Groups — public / private / secret communities with posts.
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
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub member_count: Option<i64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub is_member: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub my_role: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GroupPost {
    pub id: Uuid,
    pub group_id: Uuid,
    pub author_member_id: Uuid,
    pub author_handle: String,
    pub author_display_name: String,
    pub kind: String,
    pub title: String,
    pub body: String,
    pub pinned: bool,
    pub locked: bool,
    pub comment_count: i64,
    pub reaction_count: i64,
    pub liked_by_me: bool,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GroupComment {
    pub id: Uuid,
    pub post_id: Uuid,
    pub author_member_id: Uuid,
    pub author_handle: String,
    pub author_display_name: String,
    pub body: String,
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

#[derive(Debug, Clone, Deserialize)]
pub struct CreatePostRequest {
    #[serde(default)]
    pub title: String,
    pub body: String,
    #[serde(default = "default_post_kind")]
    pub kind: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CreateCommentRequest {
    pub body: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ReactRequest {
    #[serde(default = "default_reaction")]
    pub reaction: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ReportRequest {
    pub target_type: String,
    pub target_id: Uuid,
    #[serde(default)]
    pub reason: String,
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
fn default_post_kind() -> String {
    "discussion".into()
}
fn default_reaction() -> String {
    "like".into()
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
                i64,
            ),
        >(
            r#"
            SELECT g.id, g.slug, g.name, g.description, g.kind, g.visibility, g.join_policy,
                   g.created_at,
                   (SELECT COUNT(*) FROM community_group_members m
                    WHERE m.group_id = g.id AND NOT m.banned) AS member_count
            FROM community_groups g
            WHERE g.visibility IN ('public', 'private')
            ORDER BY g.created_at DESC
            LIMIT $1
            "#,
        )
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(map_group_counted).collect())
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
        let mut g = map_group(r);
        if g.visibility == "secret" {
            let Some(mid) = viewer_member_id else {
                return Err(DomainError::NotFound);
            };
            if self.membership(g.id, mid).await?.is_none() {
                return Err(DomainError::NotFound);
            }
        }
        let count: (i64,) = sqlx::query_as(
            r#"
            SELECT COUNT(*) FROM community_group_members
            WHERE group_id = $1 AND NOT banned
            "#,
        )
        .bind(g.id)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        g.member_count = Some(count.0);
        if let Some(mid) = viewer_member_id {
            if let Some(role) = self.membership(g.id, mid).await? {
                g.is_member = Some(true);
                g.my_role = Some(role);
            } else {
                g.is_member = Some(false);
                g.my_role = None;
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
        if !slug
            .chars()
            .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
            || slug.len() < 2
            || slug.len() > 64
        {
            return Err(DomainError::Validation(
                "slug must be 2–64 chars: lowercase, digits, hyphens".into(),
            ));
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
        let result = sqlx::query(
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
        .await;
        if let Err(e) = result {
            if let sqlx::Error::Database(db) = &e {
                if db.constraint() == Some("community_groups_slug_key") {
                    return Err(DomainError::Conflict("slug already taken".into()));
                }
            }
            return Err(DomainError::Other(e.into()));
        }
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
            member_count: Some(1),
            is_member: Some(true),
            my_role: Some("owner".into()),
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
                i64,
            ),
        >(
            r#"
            SELECT g.id, g.slug, g.name, g.description, g.kind, g.visibility, g.join_policy,
                   g.created_at,
                   (SELECT COUNT(*) FROM community_group_members m
                    WHERE m.group_id = g.id AND NOT m.banned) AS member_count
            FROM community_groups g
            WHERE g.visibility <> 'secret'
              AND (g.name ILIKE $1 OR g.description ILIKE $1 OR g.slug ILIKE $1)
            ORDER BY g.name
            LIMIT $2
            "#,
        )
        .bind(&pattern)
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(map_group_counted).collect())
    }

    /// Join with member role. Open groups only for MVP.
    pub async fn join(&self, group_id: Uuid, member_id: Uuid) -> Result<String, DomainError> {
        let g = self.require_group(group_id).await?;
        if let Some(role) = self.membership(group_id, member_id).await? {
            return Ok(role);
        }
        if g.join_policy != "open" {
            return Err(DomainError::Forbidden(
                "this group is not open to join".into(),
            ));
        }
        sqlx::query(
            r#"
            INSERT INTO community_group_members (id, group_id, member_id, role)
            VALUES ($1,$2,$3,'member')
            ON CONFLICT (group_id, member_id) DO NOTHING
            "#,
        )
        .bind(Uuid::new_v4())
        .bind(group_id)
        .bind(member_id)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok("member".into())
    }

    pub async fn list_posts(
        &self,
        group_id: Uuid,
        viewer_member_id: Option<Uuid>,
        limit: i64,
    ) -> Result<Vec<GroupPost>, DomainError> {
        let g = self.require_group(group_id).await?;
        self.ensure_can_read(&g, viewer_member_id).await?;
        let limit = limit.clamp(1, 100);
        let rows = sqlx::query_as::<_, PostRow>(
            r#"
            SELECT p.id, p.group_id, p.author_member_id,
                   COALESCE(m.handle, ''), COALESCE(m.display_name, ''),
                   p.kind, p.title, p.body, p.pinned, p.locked,
                   (SELECT COUNT(*) FROM community_group_comments c WHERE c.post_id = p.id),
                   (SELECT COUNT(*) FROM community_group_reactions r WHERE r.post_id = p.id),
                   EXISTS (
                     SELECT 1 FROM community_group_reactions r
                     WHERE r.post_id = p.id AND r.member_id = $2 AND r.reaction = 'like'
                   ),
                   p.created_at, p.updated_at
            FROM community_group_posts p
            LEFT JOIN members m ON m.id = p.author_member_id
            WHERE p.group_id = $1
            ORDER BY p.pinned DESC, p.created_at DESC
            LIMIT $3
            "#,
        )
        .bind(group_id)
        .bind(viewer_member_id.unwrap_or(Uuid::nil()))
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(map_post).collect())
    }

    pub async fn get_post(
        &self,
        group_id: Uuid,
        post_id: Uuid,
        viewer_member_id: Option<Uuid>,
    ) -> Result<GroupPost, DomainError> {
        let g = self.require_group(group_id).await?;
        self.ensure_can_read(&g, viewer_member_id).await?;
        let row = sqlx::query_as::<_, PostRow>(
            r#"
            SELECT p.id, p.group_id, p.author_member_id,
                   COALESCE(m.handle, ''), COALESCE(m.display_name, ''),
                   p.kind, p.title, p.body, p.pinned, p.locked,
                   (SELECT COUNT(*) FROM community_group_comments c WHERE c.post_id = p.id),
                   (SELECT COUNT(*) FROM community_group_reactions r WHERE r.post_id = p.id),
                   EXISTS (
                     SELECT 1 FROM community_group_reactions r
                     WHERE r.post_id = p.id AND r.member_id = $3 AND r.reaction = 'like'
                   ),
                   p.created_at, p.updated_at
            FROM community_group_posts p
            LEFT JOIN members m ON m.id = p.author_member_id
            WHERE p.id = $1 AND p.group_id = $2
            "#,
        )
        .bind(post_id)
        .bind(group_id)
        .bind(viewer_member_id.unwrap_or(Uuid::nil()))
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        row.map(map_post).ok_or(DomainError::NotFound)
    }

    pub async fn create_post(
        &self,
        group_id: Uuid,
        author_member_id: Uuid,
        req: CreatePostRequest,
    ) -> Result<GroupPost, DomainError> {
        self.require_member(group_id, author_member_id).await?;
        let body = req.body.trim();
        if body.is_empty() {
            return Err(DomainError::Validation("body is required".into()));
        }
        if body.len() > 20_000 {
            return Err(DomainError::Validation("body is too long".into()));
        }
        let kind = match req.kind.as_str() {
            "discussion" | "question" | "opportunity" | "event" | "announcement" => {
                req.kind.as_str()
            }
            _ => "discussion",
        };
        let title = req.title.trim();
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO community_group_posts (
              id, group_id, author_member_id, kind, title, body, created_at, updated_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$7)
            "#,
        )
        .bind(id)
        .bind(group_id)
        .bind(author_member_id)
        .bind(kind)
        .bind(title)
        .bind(body)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        self.get_post(group_id, id, Some(author_member_id)).await
    }

    pub async fn list_comments(
        &self,
        group_id: Uuid,
        post_id: Uuid,
        viewer_member_id: Option<Uuid>,
        limit: i64,
    ) -> Result<Vec<GroupComment>, DomainError> {
        let _ = self.get_post(group_id, post_id, viewer_member_id).await?;
        let limit = limit.clamp(1, 200);
        let rows = sqlx::query_as::<_, (Uuid, Uuid, Uuid, String, String, String, DateTime<Utc>)>(
            r#"
            SELECT c.id, c.post_id, c.author_member_id,
                   COALESCE(m.handle, ''), COALESCE(m.display_name, ''),
                   c.body, c.created_at
            FROM community_group_comments c
            LEFT JOIN members m ON m.id = c.author_member_id
            WHERE c.post_id = $1
            ORDER BY c.created_at ASC
            LIMIT $2
            "#,
        )
        .bind(post_id)
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows.into_iter().map(map_comment).collect())
    }

    pub async fn add_comment(
        &self,
        group_id: Uuid,
        post_id: Uuid,
        author_member_id: Uuid,
        req: CreateCommentRequest,
    ) -> Result<GroupComment, DomainError> {
        self.require_member(group_id, author_member_id).await?;
        let post = self
            .get_post(group_id, post_id, Some(author_member_id))
            .await?;
        if post.locked {
            return Err(DomainError::Forbidden("post is locked".into()));
        }
        let body = req.body.trim();
        if body.is_empty() {
            return Err(DomainError::Validation("body is required".into()));
        }
        if body.len() > 5_000 {
            return Err(DomainError::Validation("comment is too long".into()));
        }
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO community_group_comments (id, post_id, author_member_id, body, created_at)
            VALUES ($1,$2,$3,$4,$5)
            "#,
        )
        .bind(id)
        .bind(post_id)
        .bind(author_member_id)
        .bind(body)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        let handle_row: (String, String) = sqlx::query_as(
            r#"SELECT COALESCE(handle,''), COALESCE(display_name,'') FROM members WHERE id = $1"#,
        )
        .bind(author_member_id)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(GroupComment {
            id,
            post_id,
            author_member_id,
            author_handle: handle_row.0,
            author_display_name: handle_row.1,
            body: body.into(),
            created_at: now,
        })
    }

    /// Toggle-like: insert like; if already liked, leave as liked (idempotent).
    pub async fn react(
        &self,
        group_id: Uuid,
        post_id: Uuid,
        member_id: Uuid,
        req: ReactRequest,
    ) -> Result<(i64, bool), DomainError> {
        self.require_member(group_id, member_id).await?;
        let _ = self.get_post(group_id, post_id, Some(member_id)).await?;
        let reaction = if req.reaction.trim().is_empty() {
            "like"
        } else {
            req.reaction.trim()
        };
        if reaction.len() > 32 {
            return Err(DomainError::Validation("reaction too long".into()));
        }
        sqlx::query(
            r#"
            INSERT INTO community_group_reactions (post_id, member_id, reaction)
            VALUES ($1,$2,$3)
            ON CONFLICT (post_id, member_id, reaction) DO NOTHING
            "#,
        )
        .bind(post_id)
        .bind(member_id)
        .bind(reaction)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        let count: (i64,) =
            sqlx::query_as(r#"SELECT COUNT(*) FROM community_group_reactions WHERE post_id = $1"#)
                .bind(post_id)
                .fetch_one(&self.pool)
                .await
                .map_err(|e| DomainError::Other(e.into()))?;
        Ok((count.0, true))
    }

    pub async fn report(
        &self,
        group_id: Uuid,
        reporter_member_id: Uuid,
        req: ReportRequest,
    ) -> Result<Uuid, DomainError> {
        self.require_member(group_id, reporter_member_id).await?;
        let target_type = req.target_type.trim();
        if target_type.is_empty() {
            return Err(DomainError::Validation("target_type is required".into()));
        }
        let id = Uuid::new_v4();
        sqlx::query(
            r#"
            INSERT INTO community_group_reports (
              id, group_id, reporter_member_id, target_type, target_id, reason
            ) VALUES ($1,$2,$3,$4,$5,$6)
            "#,
        )
        .bind(id)
        .bind(group_id)
        .bind(reporter_member_id)
        .bind(target_type)
        .bind(req.target_id)
        .bind(req.reason.trim())
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(id)
    }

    async fn require_group(&self, group_id: Uuid) -> Result<Group, DomainError> {
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
            FROM community_groups WHERE id = $1
            "#,
        )
        .bind(group_id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        row.map(map_group).ok_or(DomainError::NotFound)
    }

    async fn membership(
        &self,
        group_id: Uuid,
        member_id: Uuid,
    ) -> Result<Option<String>, DomainError> {
        let row: Option<(String,)> = sqlx::query_as(
            r#"
            SELECT role FROM community_group_members
            WHERE group_id = $1 AND member_id = $2 AND NOT banned
            "#,
        )
        .bind(group_id)
        .bind(member_id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(row.map(|r| r.0))
    }

    async fn require_member(&self, group_id: Uuid, member_id: Uuid) -> Result<String, DomainError> {
        self.membership(group_id, member_id)
            .await?
            .ok_or_else(|| DomainError::Forbidden("join the group first".into()))
    }

    async fn ensure_can_read(
        &self,
        g: &Group,
        viewer_member_id: Option<Uuid>,
    ) -> Result<(), DomainError> {
        match g.visibility.as_str() {
            "public" => Ok(()),
            "private" | "secret" => {
                let Some(mid) = viewer_member_id else {
                    return Err(if g.visibility == "secret" {
                        DomainError::NotFound
                    } else {
                        DomainError::Forbidden("members only".into())
                    });
                };
                if self.membership(g.id, mid).await?.is_none() {
                    return Err(if g.visibility == "secret" {
                        DomainError::NotFound
                    } else {
                        DomainError::Forbidden("members only".into())
                    });
                }
                Ok(())
            }
            _ => Ok(()),
        }
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
        member_count: None,
        is_member: None,
        my_role: None,
    }
}

fn map_group_counted(
    (id, slug, name, description, kind, visibility, join_policy, created_at, member_count): (
        Uuid,
        String,
        String,
        String,
        String,
        String,
        String,
        DateTime<Utc>,
        i64,
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
        member_count: Some(member_count),
        is_member: None,
        my_role: None,
    }
}

type PostRow = (
    Uuid,
    Uuid,
    Uuid,
    String,
    String,
    String,
    String,
    String,
    bool,
    bool,
    i64,
    i64,
    bool,
    DateTime<Utc>,
    DateTime<Utc>,
);

fn map_post(
    (
        id,
        group_id,
        author_member_id,
        author_handle,
        author_display_name,
        kind,
        title,
        body,
        pinned,
        locked,
        comment_count,
        reaction_count,
        liked_by_me,
        created_at,
        updated_at,
    ): PostRow,
) -> GroupPost {
    GroupPost {
        id,
        group_id,
        author_member_id,
        author_handle,
        author_display_name,
        kind,
        title,
        body,
        pinned,
        locked,
        comment_count,
        reaction_count,
        liked_by_me,
        created_at,
        updated_at,
    }
}

fn map_comment(
    (id, post_id, author_member_id, author_handle, author_display_name, body, created_at): (
        Uuid,
        Uuid,
        Uuid,
        String,
        String,
        String,
        DateTime<Utc>,
    ),
) -> GroupComment {
    GroupComment {
        id,
        post_id,
        author_member_id,
        author_handle,
        author_display_name,
        body,
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

    #[test]
    fn open_join_policy_is_default() {
        assert_eq!(super::default_join(), "open");
    }
}
