use std::collections::HashSet;

use chrono::{DateTime, Utc};
use jobshout_domain::{
    DomainError, FeedActor, FeedEntry, FeedInsight, FeedItem, FeedItemKind, FollowedEntry,
    InsightKind, Member, MemberId, ShowcaseKind,
};
use sqlx::postgres::PgRow;
use sqlx::{PgPool, Row};
use uuid::Uuid;

fn db(e: sqlx::Error) -> DomainError {
    DomainError::Other(e.into())
}

/// A member as `$1` (the viewer's email, or NULL) sees them. Counts only
/// public, published work.
const MEMBER: &str = r#"
    SELECT m.id, m.handle, m.email, m.display_name, m.headline, m.is_agent,
           m.follower_count, m.created_at,
           (SELECT COUNT(*) FROM member_follows f WHERE f.follower_email = m.email)
             AS following_count,
           ($1::text IS NOT NULL AND EXISTS (
             SELECT 1 FROM member_follows f
             WHERE f.member_id = m.id AND f.follower_email = lower($1))) AS following,
           (SELECT COUNT(*) FROM showcase_apps a
             WHERE lower(a.creator_email) = m.email
               AND a.status = 'published' AND a.visibility = 'public') AS entry_count,
           (SELECT COUNT(*) FROM insights_items i
             WHERE lower(i.author_email) = m.email AND i.status = 'published') AS insight_count
    FROM members m
"#;

#[derive(Clone)]
pub struct MemberRepository {
    pool: PgPool,
}

impl MemberRepository {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn by_email(
        &self,
        email: &str,
        viewer: Option<&str>,
    ) -> Result<Option<(Member, String)>, DomainError> {
        let sql = format!("{MEMBER} WHERE m.email = lower($2)");
        let row = sqlx::query(&sql)
            .bind(viewer)
            .bind(email)
            .fetch_optional(&self.pool)
            .await
            .map_err(db)?;
        row.as_ref().map(member_from_row).transpose()
    }

    pub async fn by_handle(
        &self,
        handle: &str,
        viewer: Option<&str>,
    ) -> Result<Option<(Member, String)>, DomainError> {
        let sql = format!("{MEMBER} WHERE m.handle = lower($2)");
        let row = sqlx::query(&sql)
            .bind(viewer)
            .bind(handle.trim())
            .fetch_optional(&self.pool)
            .await
            .map_err(db)?;
        row.as_ref().map(member_from_row).transpose()
    }

    pub async fn directory(
        &self,
        q: Option<&str>,
        viewer: Option<&str>,
        limit: i64,
        offset: i64,
    ) -> Result<(Vec<Member>, i64), DomainError> {
        let sql = format!(
            r#"
            WITH x AS ({MEMBER})
            SELECT x.*, COUNT(*) OVER() AS total FROM x
            WHERE x.entry_count + x.insight_count > 0
              AND ($2::text IS NULL
                   OR x.display_name ILIKE '%' || $2 || '%' ESCAPE '\'
                   OR x.handle ILIKE '%' || $2 || '%' ESCAPE '\'
                   OR x.headline ILIKE '%' || $2 || '%' ESCAPE '\')
            ORDER BY x.follower_count DESC, x.entry_count + x.insight_count DESC, x.created_at
            LIMIT $3 OFFSET $4
            "#
        );
        let rows = sqlx::query(&sql)
            .bind(viewer)
            .bind(q.map(escape_like))
            .bind(limit)
            .bind(offset)
            .fetch_all(&self.pool)
            .await
            .map_err(db)?;
        let total = rows.first().map(|r| r.get::<i64, _>("total")).unwrap_or(0);
        let members = rows
            .iter()
            .map(|r| member_from_row(r).map(|(m, _)| m))
            .collect::<Result<_, _>>()?;
        Ok((members, total))
    }

    pub async fn handles_like(&self, base: &str) -> Result<HashSet<String>, DomainError> {
        let rows: Vec<String> =
            sqlx::query_scalar("SELECT handle FROM members WHERE handle = $1 OR handle LIKE $2")
                .bind(base)
                .bind(format!("{}-%", escape_like(base)))
                .fetch_all(&self.pool)
                .await
                .map_err(db)?;
        Ok(rows.into_iter().collect())
    }

    /// Insert a member. An existing member for the email is left alone; a
    /// taken handle is a Conflict, for the caller to retry with another.
    pub async fn insert(
        &self,
        handle: &str,
        email: &str,
        name: &str,
        is_agent: bool,
    ) -> Result<(), DomainError> {
        let res = sqlx::query(
            r#"
            INSERT INTO members (id, handle, email, display_name, is_agent)
            VALUES ($1, $2, lower($3), $4, $5)
            ON CONFLICT (email) DO NOTHING
            "#,
        )
        .bind(Uuid::new_v4())
        .bind(handle)
        .bind(email)
        .bind(name)
        .bind(is_agent)
        .execute(&self.pool)
        .await;
        match res {
            Ok(_) => Ok(()),
            Err(sqlx::Error::Database(e)) if e.is_unique_violation() => {
                Err(DomainError::Conflict("handle taken".into()))
            }
            Err(e) => Err(db(e)),
        }
    }

    pub async fn rename(&self, id: MemberId, name: &str) -> Result<(), DomainError> {
        sqlx::query("UPDATE members SET display_name = $2, updated_at = NOW() WHERE id = $1")
            .bind(id)
            .bind(name)
            .execute(&self.pool)
            .await
            .map_err(db)?;
        Ok(())
    }

    pub async fn set_headline(&self, id: MemberId, headline: &str) -> Result<(), DomainError> {
        sqlx::query("UPDATE members SET headline = $2, updated_at = NOW() WHERE id = $1")
            .bind(id)
            .bind(headline)
            .execute(&self.pool)
            .await
            .map_err(db)?;
        Ok(())
    }

    /// Creators and authors with no member yet, with the name they last
    /// published under.
    pub async fn authors_without_member(&self) -> Result<Vec<(String, String)>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT DISTINCT ON (email) email, name FROM (
              SELECT lower(creator_email) AS email, creator_display_name AS name, updated_at
              FROM showcase_apps
              UNION ALL
              SELECT lower(author_email), author_display_name, updated_at
              FROM insights_items
            ) w
            WHERE email <> '' AND NOT EXISTS (SELECT 1 FROM members m WHERE m.email = w.email)
            ORDER BY email, updated_at DESC
            "#,
        )
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| (r.get("email"), r.get("name")))
            .collect())
    }

    /// Follow or unfollow and return the new follower count, recomputed in
    /// the same transaction so concurrent follows cannot drift it.
    pub async fn set_follow(
        &self,
        member: MemberId,
        follower_email: &str,
        on: bool,
    ) -> Result<i32, DomainError> {
        let mut tx = self.pool.begin().await.map_err(db)?;
        let change = if on {
            "INSERT INTO member_follows (member_id, follower_email) VALUES ($1, lower($2)) ON CONFLICT DO NOTHING"
        } else {
            "DELETE FROM member_follows WHERE member_id = $1 AND follower_email = lower($2)"
        };
        sqlx::query(change)
            .bind(member)
            .bind(follower_email.trim())
            .execute(&mut *tx)
            .await
            .map_err(db)?;
        let count: i32 = sqlx::query_scalar(
            r#"
            UPDATE members
            SET follower_count = (SELECT COUNT(*) FROM member_follows WHERE member_id = $1)
            WHERE id = $1
            RETURNING follower_count
            "#,
        )
        .bind(member)
        .fetch_one(&mut *tx)
        .await
        .map_err(db)?;
        tx.commit().await.map_err(db)?;
        Ok(count)
    }

    pub async fn followed_members(&self, email: &str) -> Result<Vec<Member>, DomainError> {
        let sql = format!(
            r#"
            {MEMBER}
            JOIN member_follows mf ON mf.member_id = m.id AND mf.follower_email = lower($1)
            ORDER BY mf.created_at DESC
            LIMIT 200
            "#
        );
        let rows = sqlx::query(&sql)
            .bind(email.trim())
            .fetch_all(&self.pool)
            .await
            .map_err(db)?;
        rows.iter()
            .map(|r| member_from_row(r).map(|(m, _)| m))
            .collect()
    }

    /// Followed entries that are still visible: published and not private.
    pub async fn followed_entries(&self, email: &str) -> Result<Vec<FollowedEntry>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT a.slug AS e_slug, a.kind AS e_kind, a.name AS e_name, a.tagline AS e_tagline,
                   a.logo_url AS e_logo, a.version AS e_version, a.follower_count
            FROM showcase_follows f JOIN showcase_apps a ON a.id = f.app_id
            WHERE f.user_email = lower($1)
              AND a.status = 'published' AND a.visibility <> 'private'
            ORDER BY f.created_at DESC
            LIMIT 200
            "#,
        )
        .bind(email.trim())
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        rows.iter()
            .map(|r| {
                Ok(FollowedEntry {
                    entry: feed_entry(r, "e")?.ok_or_else(|| bad("entry"))?,
                    follower_count: r.get("follower_count"),
                })
            })
            .collect()
    }

    /// The raw feed for `email`, newest first. Every source shows only
    /// public, published work; an entry made private drops out.
    pub async fn feed(
        &self,
        email: &str,
        since: DateTime<Utc>,
        before: Option<DateTime<Utc>>,
        limit: i64,
    ) -> Result<Vec<FeedItem>, DomainError> {
        const NO_USES: &str = "NULL::text AS u_slug, NULL::text AS u_kind, NULL::text AS u_name,
            NULL::text AS u_tagline, NULL::text AS u_logo, NULL::text AS u_version";
        const NO_INSIGHT: &str = "NULL::text AS i_slug, NULL::text AS i_kind,
            NULL::text AS i_title, NULL::text AS i_summary";
        const NO_ENTRY: &str = "NULL::text AS e_slug, NULL::text AS e_kind, NULL::text AS e_name,
            NULL::text AS e_tagline, NULL::text AS e_logo, NULL::text AS e_version";
        const NO_ACTOR: &str = "NULL::text AS actor_handle, NULL::text AS actor_name,
            NULL::bool AS actor_is_agent";
        let entry = |a: &str| {
            format!(
                "{a}.slug AS e_slug, {a}.kind AS e_kind, {a}.name AS e_name, \
                 {a}.tagline AS e_tagline, {a}.logo_url AS e_logo, {a}.version AS e_version"
            )
        };
        let actor = "m.handle AS actor_handle, m.display_name AS actor_name, \
                     m.is_agent AS actor_is_agent";
        let a_entry = entry("a");
        let u_entry = entry("u");
        let sql = format!(
            r#"
            SELECT * FROM (
              SELECT 'new_entry' AS kind, a.published_at AS at, {actor}, {a_entry},
                     {NO_USES}, {NO_INSIGHT}
              FROM member_follows f
              JOIN members m ON m.id = f.member_id
              JOIN showcase_apps a ON lower(a.creator_email) = m.email
              WHERE f.follower_email = lower($1)
                AND a.status = 'published' AND a.visibility = 'public'

              UNION ALL
              SELECT 'new_insight', i.published_at, {actor}, {NO_ENTRY}, {NO_USES},
                     i.slug, i.kind, i.title, i.summary
              FROM member_follows f
              JOIN members m ON m.id = f.member_id
              JOIN insights_items i ON lower(i.author_email) = m.email
              WHERE f.follower_email = lower($1) AND i.status = 'published'

              UNION ALL
              SELECT 'entry_updated', a.revised_at, {NO_ACTOR}, {a_entry},
                     {NO_USES}, {NO_INSIGHT}
              FROM showcase_follows sf
              JOIN showcase_apps a ON a.id = sf.app_id
              WHERE sf.user_email = lower($1)
                AND a.status = 'published' AND a.visibility = 'public'
                AND a.revised_at IS NOT NULL

              UNION ALL
              SELECT 'used_in', u.published_at, {NO_ACTOR}, {u_entry},
                     t.slug, t.kind, t.name, t.tagline, t.logo_url, t.version, {NO_INSIGHT}
              FROM showcase_follows sf
              JOIN showcase_apps t ON t.id = sf.app_id
              JOIN showcase_links l ON l.to_id = t.id
              JOIN showcase_apps u ON u.id = l.from_id
              WHERE sf.user_email = lower($1)
                AND t.status = 'published' AND t.visibility <> 'private'
                AND u.status = 'published' AND u.visibility = 'public'
            ) feed
            WHERE at IS NOT NULL AND at >= $2 AND ($3::timestamptz IS NULL OR at < $3)
            ORDER BY at DESC
            LIMIT $4
            "#
        );
        let rows = sqlx::query(&sql)
            .bind(email.trim())
            .bind(since)
            .bind(before)
            .bind(limit)
            .fetch_all(&self.pool)
            .await
            .map_err(db)?;
        rows.iter().map(feed_from_row).collect()
    }
}

fn bad(what: &str) -> DomainError {
    DomainError::Other(anyhow::anyhow!("invalid {what} in feed"))
}

/// `%`, `_` and `\` match literally in an ILIKE ... ESCAPE '\' pattern.
fn escape_like(s: &str) -> String {
    s.replace('\\', "\\\\")
        .replace('%', "\\%")
        .replace('_', "\\_")
}

fn member_from_row(r: &PgRow) -> Result<(Member, String), DomainError> {
    Ok((
        Member {
            id: r.get("id"),
            handle: r.get("handle"),
            display_name: r.get("display_name"),
            headline: r.get("headline"),
            is_agent: r.get("is_agent"),
            follower_count: r.get("follower_count"),
            following_count: r.get("following_count"),
            following: r.get("following"),
            entry_count: r.get("entry_count"),
            insight_count: r.get("insight_count"),
            created_at: r.get("created_at"),
        },
        r.get("email"),
    ))
}

/// The entry in columns `{p}_slug`, `{p}_kind`, …; None when they are NULL.
fn feed_entry(r: &PgRow, p: &str) -> Result<Option<FeedEntry>, DomainError> {
    let Some(slug) = r.get::<Option<String>, _>(format!("{p}_slug").as_str()) else {
        return Ok(None);
    };
    let col = |c: &str| r.get::<String, _>(format!("{p}_{c}").as_str());
    Ok(Some(FeedEntry {
        slug,
        kind: ShowcaseKind::parse(&col("kind")).ok_or_else(|| bad("entry kind"))?,
        name: col("name"),
        tagline: col("tagline"),
        logo_url: col("logo"),
        version: col("version"),
    }))
}

fn feed_from_row(r: &PgRow) -> Result<FeedItem, DomainError> {
    let kind = FeedItemKind::parse(r.get("kind")).ok_or_else(|| bad("kind"))?;
    let actor = r
        .get::<Option<String>, _>("actor_handle")
        .map(|handle| FeedActor {
            handle,
            display_name: r.get("actor_name"),
            is_agent: r.get("actor_is_agent"),
        });
    let insight = match r.get::<Option<String>, _>("i_slug") {
        None => None,
        Some(slug) => Some(FeedInsight {
            slug,
            kind: InsightKind::parse(r.get("i_kind")).ok_or_else(|| bad("insight kind"))?,
            title: r.get("i_title"),
            summary: r.get("i_summary"),
        }),
    };
    Ok(FeedItem {
        kind,
        at: r.get("at"),
        actor,
        entry: feed_entry(r, "e")?,
        uses: feed_entry(r, "u")?,
        insight,
    })
}
