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
           COALESCE(m.bio, '') AS bio,
           COALESCE(m.avatar_url, '') AS avatar_url,
           COALESCE(m.location, '') AS location,
           COALESCE(m.timezone, '') AS timezone,
           COALESCE(m.availability, '') AS availability,
           m.rate_min::float8 AS rate_min,
           m.rate_max::float8 AS rate_max,
           COALESCE(m.rate_currency, 'GBP') AS rate_currency,
           COALESCE(m.languages, '{}') AS languages,
           COALESCE(m.links, '[]'::jsonb) AS links,
           COALESCE(m.services_offered, '{}') AS services_offered,
           COALESCE(m.open_to_work, false) AS open_to_work,
           COALESCE(m.open_to_hire, false) AS open_to_hire,
           COALESCE(m.connection_count, 0) AS connection_count,
           (SELECT COUNT(*) FROM member_follows f WHERE f.follower_email = m.email
              OR f.follower_member_id = m.id)
             AS following_count,
           ($1::text IS NOT NULL AND EXISTS (
             SELECT 1 FROM member_follows f
             WHERE f.member_id = m.id
               AND (f.follower_email = lower($1)
                    OR f.follower_member_id = (
                         SELECT id FROM members WHERE email = lower($1) LIMIT 1)))) AS following,
           ($1::text IS NOT NULL AND EXISTS (
             SELECT 1 FROM member_connections c
             WHERE c.status = 'accepted'
               AND (
                 (c.requester_id = m.id AND c.addressee_id = (
                    SELECT id FROM members WHERE email = lower($1) LIMIT 1))
                 OR (c.addressee_id = m.id AND c.requester_id = (
                    SELECT id FROM members WHERE email = lower($1) LIMIT 1))
               ))) AS connected,
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

    pub async fn update_rich_profile(
        &self,
        id: MemberId,
        input: &jobshout_domain::MemberInput,
        headline: &str,
    ) -> Result<(), DomainError> {
        // Read-modify-write so omitted Option fields keep current values.
        let row = sqlx::query(
            r#"
            SELECT bio, avatar_url, location, timezone, availability,
                   rate_min::float8 AS rate_min, rate_max::float8 AS rate_max,
                   rate_currency, languages, links, services_offered,
                   open_to_work, open_to_hire, hide_from_employer
            FROM members WHERE id = $1
            "#,
        )
        .bind(id)
        .fetch_one(&self.pool)
        .await
        .map_err(db)?;

        let bio: String = input.bio.clone().unwrap_or_else(|| row.get("bio"));
        let avatar_url: String = input
            .avatar_url
            .clone()
            .unwrap_or_else(|| row.get("avatar_url"));
        let location: String = input
            .location
            .clone()
            .unwrap_or_else(|| row.get("location"));
        let timezone: String = input
            .timezone
            .clone()
            .unwrap_or_else(|| row.get("timezone"));
        let availability: String = input
            .availability
            .clone()
            .unwrap_or_else(|| row.get("availability"));
        let rate_min: Option<f64> = match &input.rate_min {
            Some(v) => *v,
            None => row.try_get("rate_min").ok().flatten(),
        };
        let rate_max: Option<f64> = match &input.rate_max {
            Some(v) => *v,
            None => row.try_get("rate_max").ok().flatten(),
        };
        let rate_currency: String = input
            .rate_currency
            .clone()
            .unwrap_or_else(|| row.get("rate_currency"));
        let languages: Vec<String> = input
            .languages
            .clone()
            .unwrap_or_else(|| row.get("languages"));
        let links = match &input.links {
            Some(links) => serde_json::to_value(links).unwrap_or(serde_json::json!([])),
            None => row.try_get("links").unwrap_or(serde_json::json!([])),
        };
        let services: Vec<String> = input
            .services_offered
            .clone()
            .unwrap_or_else(|| row.get("services_offered"));
        let open_to_work = input
            .open_to_work
            .unwrap_or_else(|| row.get("open_to_work"));
        let open_to_hire = input
            .open_to_hire
            .unwrap_or_else(|| row.get("open_to_hire"));
        let hide = input
            .hide_from_employer
            .unwrap_or_else(|| row.get("hide_from_employer"));

        sqlx::query(
            r#"
            UPDATE members SET
              headline = $2, bio = $3, avatar_url = $4, location = $5, timezone = $6,
              availability = $7, rate_min = $8, rate_max = $9, rate_currency = $10,
              languages = $11, links = $12, services_offered = $13,
              open_to_work = $14, open_to_hire = $15, hide_from_employer = $16,
              updated_at = NOW()
            WHERE id = $1
            "#,
        )
        .bind(id)
        .bind(headline)
        .bind(bio.chars().take(4000).collect::<String>())
        .bind(avatar_url.chars().take(2000).collect::<String>())
        .bind(location.chars().take(200).collect::<String>())
        .bind(timezone.chars().take(64).collect::<String>())
        .bind(availability.chars().take(32).collect::<String>())
        .bind(rate_min)
        .bind(rate_max)
        .bind(rate_currency.chars().take(8).collect::<String>())
        .bind(languages)
        .bind(links)
        .bind(services)
        .bind(open_to_work)
        .bind(open_to_hire)
        .bind(hide)
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(())
    }

    pub async fn list_experience(
        &self,
        member_id: MemberId,
    ) -> Result<Vec<jobshout_domain::MemberExperience>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT id, title, organisation, location, started_on, ended_on,
                   is_current, summary, sort_order
            FROM member_experience WHERE member_id = $1
            ORDER BY sort_order, started_on DESC NULLS LAST
            "#,
        )
        .bind(member_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| jobshout_domain::MemberExperience {
                id: r.get("id"),
                title: r.get("title"),
                organisation: r.get("organisation"),
                location: r.get("location"),
                started_on: r.get("started_on"),
                ended_on: r.get("ended_on"),
                is_current: r.get("is_current"),
                summary: r.get("summary"),
                sort_order: r.get("sort_order"),
            })
            .collect())
    }

    pub async fn list_education(
        &self,
        member_id: MemberId,
    ) -> Result<Vec<jobshout_domain::MemberEducation>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT id, school, degree, field_of_study, started_on, ended_on, summary, sort_order
            FROM member_education WHERE member_id = $1
            ORDER BY sort_order, started_on DESC NULLS LAST
            "#,
        )
        .bind(member_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| jobshout_domain::MemberEducation {
                id: r.get("id"),
                school: r.get("school"),
                degree: r.get("degree"),
                field_of_study: r.get("field_of_study"),
                started_on: r.get("started_on"),
                ended_on: r.get("ended_on"),
                summary: r.get("summary"),
                sort_order: r.get("sort_order"),
            })
            .collect())
    }

    pub async fn list_certifications(
        &self,
        member_id: MemberId,
    ) -> Result<Vec<jobshout_domain::MemberCertification>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT id, name, issuer, issued_on, expires_on, credential_url, sort_order
            FROM member_certifications WHERE member_id = $1
            ORDER BY sort_order, issued_on DESC NULLS LAST
            "#,
        )
        .bind(member_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| jobshout_domain::MemberCertification {
                id: r.get("id"),
                name: r.get("name"),
                issuer: r.get("issuer"),
                issued_on: r.get("issued_on"),
                expires_on: r.get("expires_on"),
                credential_url: r.get("credential_url"),
                sort_order: r.get("sort_order"),
            })
            .collect())
    }

    pub async fn list_portfolio(
        &self,
        member_id: MemberId,
    ) -> Result<Vec<jobshout_domain::MemberPortfolioLink>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT id, title, url, kind, showcase_slug, sort_order
            FROM member_portfolio_links WHERE member_id = $1
            ORDER BY sort_order, created_at
            "#,
        )
        .bind(member_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| jobshout_domain::MemberPortfolioLink {
                id: r.get("id"),
                title: r.get("title"),
                url: r.get("url"),
                kind: r.get("kind"),
                showcase_slug: r.get("showcase_slug"),
                sort_order: r.get("sort_order"),
            })
            .collect())
    }

    pub async fn list_skills(
        &self,
        member_id: MemberId,
    ) -> Result<Vec<jobshout_domain::MemberSkill>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT id, skill_name, evidence_level, proficiency, endorsement_count
            FROM member_skills WHERE member_id = $1
            ORDER BY endorsement_count DESC, skill_name
            "#,
        )
        .bind(member_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        rows.iter()
            .map(|r| {
                let level = r.get::<String, _>("evidence_level");
                Ok(jobshout_domain::MemberSkill {
                    id: r.get("id"),
                    skill_name: r.get("skill_name"),
                    evidence_level: jobshout_domain::SkillEvidence::parse(&level)
                        .ok_or_else(|| DomainError::Other(anyhow::anyhow!("bad evidence_level")))?,
                    proficiency: r.get("proficiency"),
                    endorsement_count: r.get("endorsement_count"),
                })
            })
            .collect()
    }

    pub async fn list_reviews(
        &self,
        member_id: MemberId,
    ) -> Result<Vec<jobshout_domain::MemberReview>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT r.id, r.rating, r.body, r.created_at,
                   a.handle AS author_handle, a.display_name AS author_display_name
            FROM member_reviews r
            JOIN members a ON a.id = r.author_id
            WHERE r.subject_id = $1
            ORDER BY r.created_at DESC
            LIMIT 50
            "#,
        )
        .bind(member_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| jobshout_domain::MemberReview {
                id: r.get("id"),
                rating: r.get("rating"),
                body: r.get("body"),
                author_handle: r.get("author_handle"),
                author_display_name: r.get("author_display_name"),
                created_at: r.get("created_at"),
            })
            .collect())
    }

    pub async fn list_achievements(
        &self,
        member_id: MemberId,
    ) -> Result<Vec<jobshout_domain::MemberAchievement>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT kind, title, detail, earned_at
            FROM member_achievements WHERE member_id = $1
            ORDER BY earned_at DESC
            "#,
        )
        .bind(member_id)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| jobshout_domain::MemberAchievement {
                kind: r.get("kind"),
                title: r.get("title"),
                detail: r.get("detail"),
                earned_at: r.get("earned_at"),
            })
            .collect())
    }

    pub async fn request_connection(
        &self,
        requester: MemberId,
        addressee: MemberId,
    ) -> Result<jobshout_domain::ConnectionStatus, DomainError> {
        if requester == addressee {
            return Err(DomainError::Validation(
                "you can't connect with yourself".into(),
            ));
        }
        sqlx::query(
            r#"
            INSERT INTO member_connections (requester_id, addressee_id, status)
            VALUES ($1, $2, 'pending')
            ON CONFLICT (requester_id, addressee_id) DO NOTHING
            "#,
        )
        .bind(requester)
        .bind(addressee)
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(jobshout_domain::ConnectionStatus::Pending)
    }

    pub async fn respond_connection(
        &self,
        addressee: MemberId,
        requester: MemberId,
        accept: bool,
    ) -> Result<jobshout_domain::ConnectionStatus, DomainError> {
        let status = if accept { "accepted" } else { "declined" };
        let mut tx = self.pool.begin().await.map_err(db)?;
        let updated = sqlx::query(
            r#"
            UPDATE member_connections
            SET status = $3, updated_at = NOW()
            WHERE requester_id = $1 AND addressee_id = $2 AND status = 'pending'
            "#,
        )
        .bind(requester)
        .bind(addressee)
        .bind(status)
        .execute(&mut *tx)
        .await
        .map_err(db)?;
        if updated.rows_affected() == 0 {
            return Err(DomainError::NotFound);
        }
        if accept {
            for id in [requester, addressee] {
                sqlx::query(
                    r#"
                    UPDATE members SET connection_count = (
                      SELECT COUNT(*)::int FROM member_connections c
                      WHERE c.status = 'accepted'
                        AND (c.requester_id = $1 OR c.addressee_id = $1)
                    ), updated_at = NOW()
                    WHERE id = $1
                    "#,
                )
                .bind(id)
                .execute(&mut *tx)
                .await
                .map_err(db)?;
            }
        }
        tx.commit().await.map_err(db)?;
        Ok(if accept {
            jobshout_domain::ConnectionStatus::Accepted
        } else {
            jobshout_domain::ConnectionStatus::Declined
        })
    }

    pub async fn talent_search(
        &self,
        query: &jobshout_domain::TalentSearchQuery,
        q: Option<&str>,
        availability: Option<&str>,
        viewer: Option<&str>,
    ) -> Result<(Vec<Member>, i64), DomainError> {
        let limit = query.limit.clamp(1, 50);
        let offset = query.offset.max(0);
        let sql = format!(
            r#"
            WITH x AS ({MEMBER})
            SELECT x.*, COUNT(*) OVER() AS total FROM x
            WHERE COALESCE(
                    (SELECT searchable FROM member_privacy_settings p WHERE p.member_id = x.id),
                    true)
              AND ($2::text IS NULL
                   OR x.display_name ILIKE '%' || $2 || '%' ESCAPE '\'
                   OR x.handle ILIKE '%' || $2 || '%' ESCAPE '\'
                   OR x.headline ILIKE '%' || $2 || '%' ESCAPE '\'
                   OR x.bio ILIKE '%' || $2 || '%' ESCAPE '\'
                   OR EXISTS (
                        SELECT 1 FROM member_skills s
                        WHERE s.member_id = x.id
                          AND s.skill_name ILIKE '%' || $2 || '%' ESCAPE '\'))
              AND ($3::text IS NULL OR x.availability = $3)
              AND ($4::bool IS NULL OR x.open_to_work = $4)
              AND ($5::bool IS NULL OR x.is_agent = $5)
            ORDER BY x.follower_count DESC, x.connection_count DESC, x.created_at
            LIMIT $6 OFFSET $7
            "#
        );
        let rows = sqlx::query(&sql)
            .bind(viewer)
            .bind(q.map(escape_like))
            .bind(availability)
            .bind(query.open_to_work)
            .bind(query.is_agent)
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

    pub async fn dashboard_layout(&self, member_id: MemberId) -> Result<Vec<String>, DomainError> {
        let row: Option<serde_json::Value> =
            sqlx::query_scalar("SELECT widgets FROM dashboard_layouts WHERE member_id = $1")
                .bind(member_id)
                .fetch_optional(&self.pool)
                .await
                .map_err(db)?;
        Ok(match row {
            Some(v) => serde_json::from_value(v).unwrap_or_default(),
            None => default_dashboard_widgets(),
        })
    }

    pub async fn save_dashboard_layout(
        &self,
        member_id: MemberId,
        widgets: &[String],
    ) -> Result<(), DomainError> {
        let value = serde_json::to_value(widgets).unwrap_or(serde_json::json!([]));
        sqlx::query(
            r#"
            INSERT INTO dashboard_layouts (member_id, widgets)
            VALUES ($1, $2)
            ON CONFLICT (member_id) DO UPDATE
              SET widgets = EXCLUDED.widgets, updated_at = NOW()
            "#,
        )
        .bind(member_id)
        .bind(value)
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(())
    }

    pub async fn create_gdpr_request(
        &self,
        member_id: MemberId,
        kind: &str,
    ) -> Result<Uuid, DomainError> {
        let id: Uuid = sqlx::query_scalar(
            r#"
            INSERT INTO gdpr_requests (member_id, kind, status)
            VALUES ($1, $2, 'pending')
            RETURNING id
            "#,
        )
        .bind(member_id)
        .bind(kind)
        .fetch_one(&self.pool)
        .await
        .map_err(db)?;
        Ok(id)
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
    ///
    /// Dual-writes `follower_email` (legacy) and `follower_member_id` (Phase 0).
    pub async fn set_follow(
        &self,
        member: MemberId,
        follower_email: &str,
        on: bool,
    ) -> Result<i32, DomainError> {
        let mut tx = self.pool.begin().await.map_err(db)?;
        let follower_member_id: Option<Uuid> =
            sqlx::query_scalar("SELECT id FROM members WHERE email = lower($1) LIMIT 1")
                .bind(follower_email.trim())
                .fetch_optional(&mut *tx)
                .await
                .map_err(db)?;
        if on {
            sqlx::query(
                r#"
                INSERT INTO member_follows (member_id, follower_email, follower_member_id)
                VALUES ($1, lower($2), $3)
                ON CONFLICT (member_id, follower_email) DO UPDATE
                  SET follower_member_id = COALESCE(EXCLUDED.follower_member_id, member_follows.follower_member_id)
                "#,
            )
            .bind(member)
            .bind(follower_email.trim())
            .bind(follower_member_id)
            .execute(&mut *tx)
            .await
            .map_err(db)?;
        } else {
            sqlx::query(
                r#"
                DELETE FROM member_follows
                WHERE member_id = $1
                  AND (follower_email = lower($2)
                       OR ($3::uuid IS NOT NULL AND follower_member_id = $3))
                "#,
            )
            .bind(member)
            .bind(follower_email.trim())
            .bind(follower_member_id)
            .execute(&mut *tx)
            .await
            .map_err(db)?;
        }
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
            JOIN member_follows mf ON mf.member_id = m.id
              AND (mf.follower_email = lower($1)
                   OR mf.follower_member_id = (SELECT id FROM members WHERE email = lower($1) LIMIT 1))
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
            WHERE (f.user_email = lower($1)
                   OR f.follower_member_id = (SELECT id FROM members WHERE email = lower($1) LIMIT 1))
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
              WHERE (f.follower_email = lower($1) OR f.follower_member_id = (SELECT id FROM members WHERE email = lower($1) LIMIT 1))
                AND a.status = 'published' AND a.visibility = 'public'

              UNION ALL
              SELECT 'new_insight', i.published_at, {actor}, {NO_ENTRY}, {NO_USES},
                     i.slug, i.kind, i.title, i.summary
              FROM member_follows f
              JOIN members m ON m.id = f.member_id
              JOIN insights_items i ON lower(i.author_email) = m.email
              WHERE (f.follower_email = lower($1) OR f.follower_member_id = (SELECT id FROM members WHERE email = lower($1) LIMIT 1)) AND i.status = 'published'

              UNION ALL
              SELECT 'entry_updated', a.revised_at, {NO_ACTOR}, {a_entry},
                     {NO_USES}, {NO_INSIGHT}
              FROM showcase_follows sf
              JOIN showcase_apps a ON a.id = sf.app_id
              WHERE (sf.user_email = lower($1) OR sf.follower_member_id = (SELECT id FROM members WHERE email = lower($1) LIMIT 1))
                AND a.status = 'published' AND a.visibility = 'public'
                AND a.revised_at IS NOT NULL

              UNION ALL
              SELECT 'used_in', u.published_at, {NO_ACTOR}, {u_entry},
                     t.slug, t.kind, t.name, t.tagline, t.logo_url, t.version, {NO_INSIGHT}
              FROM showcase_follows sf
              JOIN showcase_apps t ON t.id = sf.app_id
              JOIN showcase_links l ON l.to_id = t.id
              JOIN showcase_apps u ON u.id = l.from_id
              WHERE (sf.user_email = lower($1) OR sf.follower_member_id = (SELECT id FROM members WHERE email = lower($1) LIMIT 1))
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

fn default_dashboard_widgets() -> Vec<String> {
    vec![
        "today".into(),
        "opportunities".into(),
        "pipeline".into(),
        "stats".into(),
        "credits".into(),
        "messages".into(),
        "approvals".into(),
        "goals".into(),
    ]
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
    let links: serde_json::Value = r.try_get("links").unwrap_or(serde_json::json!([]));
    let links: Vec<jobshout_domain::MemberLink> = serde_json::from_value(links).unwrap_or_default();
    Ok((
        Member {
            id: r.get("id"),
            handle: r.get("handle"),
            display_name: r.get("display_name"),
            headline: r.get("headline"),
            is_agent: r.get("is_agent"),
            follower_count: r.get("follower_count"),
            following_count: r.get("following_count"),
            following: r.try_get("following").unwrap_or(false),
            connected: r.try_get("connected").unwrap_or(false),
            connection_count: r.try_get("connection_count").unwrap_or(0),
            entry_count: r.get("entry_count"),
            insight_count: r.get("insight_count"),
            bio: r.try_get("bio").unwrap_or_default(),
            avatar_url: r.try_get("avatar_url").unwrap_or_default(),
            location: r.try_get("location").unwrap_or_default(),
            timezone: r.try_get("timezone").unwrap_or_default(),
            availability: r.try_get("availability").unwrap_or_default(),
            rate_min: r.try_get("rate_min").ok().flatten(),
            rate_max: r.try_get("rate_max").ok().flatten(),
            rate_currency: r.try_get("rate_currency").unwrap_or_else(|_| "GBP".into()),
            languages: r.try_get("languages").unwrap_or_default(),
            links,
            services_offered: r.try_get("services_offered").unwrap_or_default(),
            open_to_work: r.try_get("open_to_work").unwrap_or(false),
            open_to_hire: r.try_get("open_to_hire").unwrap_or(false),
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
