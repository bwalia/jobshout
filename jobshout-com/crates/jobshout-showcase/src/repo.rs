use std::collections::{HashMap, HashSet};

use chrono::{DateTime, Utc};
use jobshout_domain::{
    DomainError, InsightSource, ShowcaseApp, ShowcaseAppId, ShowcaseAppType, ShowcaseBuildMethod,
    ShowcaseKind, ShowcaseLink, ShowcaseMaturity, ShowcasePricing, ShowcaseStatus, ShowcaseTag,
    ShowcaseVerification, ShowcaseVisibility,
};
use sqlx::types::Json;
use sqlx::{PgPool, Postgres, Row, Transaction};
use uuid::Uuid;

const COLUMNS: &str = r#"
    a.id, a.kind, a.slug, a.name, a.tagline, a.description_md, a.description_html, a.app_type,
    a.maturity, a.build_method, a.pricing, a.license, a.version, a.logo_url, a.screenshots,
    a.repo_url, a.demo_url, a.website_url, a.docs_url, a.technologies, a.ai_models, a.agents,
    a.human_oversight, a.evidence, a.team_name, a.model_provider, a.tools, a.mcp_servers,
    a.capabilities, a.creator_email, a.creator_display_name, a.visibility, a.status,
    a.verification, a.featured, a.review_note, a.star_count, a.published_at, a.created_at,
    a.updated_at
"#;

/// Public, published apps that link to `a`: directly, or (for an agent)
/// through a team that has it as a member. Zero for apps themselves.
const USED_IN: &str = r#"
    CASE WHEN a.kind = 'app' THEN 0 ELSE (
      SELECT COUNT(*) FROM showcase_apps u
      WHERE u.kind = 'app' AND u.status = 'published' AND u.visibility = 'public'
        AND (EXISTS (SELECT 1 FROM showcase_links l WHERE l.from_id = u.id AND l.to_id = a.id)
          OR EXISTS (SELECT 1 FROM showcase_links l1
                     JOIN showcase_links l2 ON l2.from_id = l1.to_id
                     WHERE l1.from_id = u.id AND l2.to_id = a.id))
    ) END
"#;

fn db(e: sqlx::Error) -> DomainError {
    DomainError::Other(e.into())
}

/// Everything a write stores. Built by the service after validation.
pub struct AppRecord {
    pub app: ShowcaseApp,
    pub source: InsightSource,
    pub tags_text: String,
    /// Link targets in display order, with the role each played.
    pub links: Vec<(ShowcaseAppId, String)>,
}

/// An entry a link points at, as resolved from a submitted slug.
#[derive(Debug, Clone)]
pub struct LinkTarget {
    pub id: ShowcaseAppId,
    pub slug: String,
    pub kind: ShowcaseKind,
    pub status: ShowcaseStatus,
    pub visibility: ShowcaseVisibility,
    pub creator_email: String,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum Sort {
    #[default]
    Newest,
    Stars,
    Updated,
}

#[derive(Debug, Default, Clone)]
pub struct ListQuery {
    /// None lists every kind (the creator's own list, the review queue).
    pub kind: Option<ShowcaseKind>,
    pub status: Option<ShowcaseStatus>,
    /// Lists and search only ever show public entries; unlisted ones are reachable by link.
    pub public_only: bool,
    pub creator_email: Option<String>,
    pub q: Option<String>,
    pub app_type: Option<ShowcaseAppType>,
    pub maturities: Vec<ShowcaseMaturity>,
    pub build_methods: Vec<ShowcaseBuildMethod>,
    pub pricing: Option<ShowcasePricing>,
    pub technology: Option<String>,
    pub capability: Option<String>,
    /// Entries that link to this one, directly or through a team.
    pub links_to: Option<ShowcaseAppId>,
    pub featured: Option<bool>,
    pub exclude: Option<ShowcaseAppId>,
    /// Marks `starred` on each result, and shows this viewer their own
    /// unpublished link targets.
    pub viewer_email: Option<String>,
    pub sort: Sort,
    pub limit: i64,
    pub offset: i64,
}

#[derive(Clone)]
pub struct ShowcaseRepository {
    pool: PgPool,
}

impl ShowcaseRepository {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn list(&self, q: &ListQuery) -> Result<(Vec<ShowcaseApp>, i64), DomainError> {
        let order = match q.sort {
            Sort::Newest => "COALESCE(a.published_at, a.updated_at) DESC, a.created_at DESC",
            Sort::Stars => "a.star_count DESC, COALESCE(a.published_at, a.updated_at) DESC",
            Sort::Updated => "a.updated_at DESC",
        };
        let sql = format!(
            r#"
            SELECT {COLUMNS}, {STARRED} AS starred, {USED_IN} AS used_in,
                   COUNT(*) OVER() AS total
            FROM showcase_apps a
            WHERE ($1::text IS NULL OR a.status = $1)
              AND (NOT $2 OR a.visibility = 'public')
              AND ($3::text IS NULL OR lower(a.creator_email) = lower($3))
              AND ($4::text IS NULL OR a.search @@ websearch_to_tsquery('english', $4))
              AND ($5::text IS NULL OR a.app_type = $5)
              AND (cardinality($6::text[]) = 0 OR a.maturity = ANY($6))
              AND (cardinality($7::text[]) = 0 OR a.build_method = ANY($7))
              AND ($8::text IS NULL OR a.pricing = $8)
              AND ($9::text IS NULL OR EXISTS (
                    SELECT 1 FROM unnest(a.technologies) t WHERE lower(t) = lower($9)))
              AND ($10::bool IS NULL OR a.featured = $10)
              AND ($11::uuid IS NULL OR a.id <> $11)
              AND ($15::text IS NULL OR a.kind = $15)
              AND ($16::text IS NULL OR $16 = ANY(a.capabilities))
              AND ($17::uuid IS NULL
                   OR EXISTS (SELECT 1 FROM showcase_links l WHERE l.from_id = a.id AND l.to_id = $17)
                   OR EXISTS (SELECT 1 FROM showcase_links l1
                              JOIN showcase_links l2 ON l2.from_id = l1.to_id
                              WHERE l1.from_id = a.id AND l2.to_id = $17))
            ORDER BY {order}
            LIMIT $13 OFFSET $14
            "#
        );
        let rows = sqlx::query(&sql)
            .bind(q.status.map(|s| s.as_str()))
            .bind(q.public_only)
            .bind(q.creator_email.as_deref())
            .bind(q.q.as_deref())
            .bind(q.app_type.map(|t| t.as_str()))
            .bind(q.maturities.iter().map(|m| m.as_str()).collect::<Vec<_>>())
            .bind(
                q.build_methods
                    .iter()
                    .map(|b| b.as_str())
                    .collect::<Vec<_>>(),
            )
            .bind(q.pricing.map(|p| p.as_str()))
            .bind(q.technology.as_deref())
            .bind(q.featured)
            .bind(q.exclude)
            .bind(q.viewer_email.as_deref())
            .bind(q.limit)
            .bind(q.offset)
            .bind(q.kind.map(|k| k.as_str()))
            .bind(q.capability.as_deref())
            .bind(q.links_to)
            .fetch_all(&self.pool)
            .await
            .map_err(db)?;
        let total = rows.first().map(|r| r.get::<i64, _>("total")).unwrap_or(0);
        let mut apps = rows.iter().map(from_row).collect::<Result<Vec<_>, _>>()?;
        self.attach_links(&mut apps, q.viewer_email.as_deref())
            .await?;
        Ok((apps, total))
    }

    pub async fn get_by_slug(
        &self,
        slug: &str,
        viewer_email: Option<&str>,
    ) -> Result<ShowcaseApp, DomainError> {
        let sql = format!(
            "SELECT {COLUMNS}, {STARRED_1} AS starred, {USED_IN} AS used_in
             FROM showcase_apps a WHERE a.slug = $2"
        );
        self.fetch_one(&sql, viewer_email, slug.to_string()).await
    }

    /// By id, as `viewer_email` sees it (their own unpublished link targets included).
    pub async fn get(
        &self,
        id: ShowcaseAppId,
        viewer_email: Option<&str>,
    ) -> Result<ShowcaseApp, DomainError> {
        let sql = format!(
            "SELECT {COLUMNS}, {STARRED_1} AS starred, {USED_IN} AS used_in
             FROM showcase_apps a WHERE a.id = $2"
        );
        self.fetch_one(&sql, viewer_email, id).await
    }

    async fn fetch_one<T>(
        &self,
        sql: &str,
        viewer_email: Option<&str>,
        key: T,
    ) -> Result<ShowcaseApp, DomainError>
    where
        T: for<'q> sqlx::Encode<'q, Postgres> + sqlx::Type<Postgres> + Send,
    {
        let row = sqlx::query(sql)
            .bind(viewer_email)
            .bind(key)
            .fetch_optional(&self.pool)
            .await
            .map_err(db)?
            .ok_or(DomainError::NotFound)?;
        let mut apps = vec![from_row(&row)?];
        self.attach_links(&mut apps, viewer_email).await?;
        Ok(apps.remove(0))
    }

    /// Fill `linked_agents` and `linked_team`. A target shows when it is
    /// published and not private, or when the viewer created it.
    async fn attach_links(
        &self,
        apps: &mut [ShowcaseApp],
        viewer_email: Option<&str>,
    ) -> Result<(), DomainError> {
        if apps.is_empty() {
            return Ok(());
        }
        let ids: Vec<Uuid> = apps.iter().map(|a| a.id).collect();
        let rows = sqlx::query(
            r#"
            SELECT l.from_id, l.role, t.slug, t.kind, t.name, t.tagline, t.logo_url
            FROM showcase_links l
            JOIN showcase_apps t ON t.id = l.to_id
            WHERE l.from_id = ANY($1)
              AND ((t.status = 'published' AND t.visibility <> 'private')
                   OR ($2::text IS NOT NULL AND lower(t.creator_email) = lower($2)))
            ORDER BY l.position, t.name
            "#,
        )
        .bind(&ids)
        .bind(viewer_email)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        let mut by_app: HashMap<Uuid, Vec<ShowcaseLink>> = HashMap::new();
        for r in &rows {
            let Some(kind) = ShowcaseKind::parse(r.get::<&str, _>("kind")) else {
                continue;
            };
            by_app
                .entry(r.get("from_id"))
                .or_default()
                .push(ShowcaseLink {
                    slug: r.get("slug"),
                    kind,
                    name: r.get("name"),
                    tagline: r.get("tagline"),
                    logo_url: r.get("logo_url"),
                    role: r.get("role"),
                });
        }
        for app in apps {
            for link in by_app.remove(&app.id).unwrap_or_default() {
                if link.kind == ShowcaseKind::Team {
                    app.linked_team = Some(link);
                } else {
                    app.linked_agents.push(link);
                }
            }
        }
        Ok(())
    }

    /// The entries these slugs name, in any status; the service decides who may link what.
    pub async fn resolve_links(&self, slugs: &[String]) -> Result<Vec<LinkTarget>, DomainError> {
        if slugs.is_empty() {
            return Ok(Vec::new());
        }
        let rows = sqlx::query(
            "SELECT id, slug, kind, status, visibility, creator_email FROM showcase_apps WHERE slug = ANY($1)",
        )
        .bind(slugs)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        let bad =
            |what: &str| DomainError::Other(anyhow::anyhow!("invalid {what} in showcase_apps"));
        rows.iter()
            .map(|r| {
                Ok(LinkTarget {
                    id: r.get("id"),
                    slug: r.get("slug"),
                    kind: ShowcaseKind::parse(r.get::<&str, _>("kind"))
                        .ok_or_else(|| bad("kind"))?,
                    status: ShowcaseStatus::parse(r.get::<&str, _>("status"))
                        .ok_or_else(|| bad("status"))?,
                    visibility: ShowcaseVisibility::parse(r.get::<&str, _>("visibility"))
                        .ok_or_else(|| bad("visibility"))?,
                    creator_email: r.get("creator_email"),
                })
            })
            .collect()
    }

    /// Technologies across public, published entries of `kind`, most used first.
    pub async fn tags(
        &self,
        kind: ShowcaseKind,
        limit: i64,
    ) -> Result<Vec<ShowcaseTag>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT min(t) AS name, COUNT(*) AS count
            FROM showcase_apps a, unnest(a.technologies) t
            WHERE a.status = 'published' AND a.visibility = 'public' AND a.kind = $2
            GROUP BY lower(t)
            ORDER BY count DESC, name
            LIMIT $1
            "#,
        )
        .bind(limit)
        .bind(kind.as_str())
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| ShowcaseTag {
                name: r.get("name"),
                count: r.get("count"),
            })
            .collect())
    }

    /// Slugs already taken that start with `base`, for picking a free one.
    pub async fn slugs_like(&self, base: &str) -> Result<HashSet<String>, DomainError> {
        let rows = sqlx::query("SELECT slug FROM showcase_apps WHERE slug = $1 OR slug LIKE $2")
            .bind(base)
            .bind(format!("{}-%", base.replace('%', "").replace('_', "\\_")))
            .fetch_all(&self.pool)
            .await
            .map_err(db)?;
        Ok(rows.iter().map(|r| r.get("slug")).collect())
    }

    pub async fn created_since(
        &self,
        email: &str,
        since: DateTime<Utc>,
    ) -> Result<i64, DomainError> {
        sqlx::query_scalar(
            "SELECT COUNT(*) FROM showcase_apps WHERE lower(creator_email) = lower($1) AND created_at >= $2",
        )
        .bind(email)
        .bind(since)
        .fetch_one(&self.pool)
        .await
        .map_err(db)
    }

    pub async fn count(&self, kind: ShowcaseKind) -> Result<i64, DomainError> {
        sqlx::query_scalar("SELECT COUNT(*) FROM showcase_apps WHERE kind = $1")
            .bind(kind.as_str())
            .fetch_one(&self.pool)
            .await
            .map_err(db)
    }

    pub async fn insert(&self, rec: &AppRecord) -> Result<(), DomainError> {
        let a = &rec.app;
        let mut tx = self.pool.begin().await.map_err(db)?;
        let res = sqlx::query(
            r#"
            INSERT INTO showcase_apps (
              id, slug, name, tagline, description_md, description_html, app_type, maturity,
              build_method, pricing, license, version, logo_url, screenshots, repo_url, demo_url,
              website_url, docs_url, technologies, ai_models, agents, human_oversight, evidence,
              team_name, creator_email, creator_display_name, visibility, status, source,
              tags_text, published_at, kind, model_provider, tools, mcp_servers, capabilities
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
                      $21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36)
            "#,
        )
        .bind(a.id)
        .bind(&a.slug)
        .bind(&a.name)
        .bind(&a.tagline)
        .bind(&a.description_md)
        .bind(&a.description_html)
        .bind(a.app_type.as_str())
        .bind(a.maturity.as_str())
        .bind(a.build_method.as_str())
        .bind(a.pricing.as_str())
        .bind(&a.license)
        .bind(&a.version)
        .bind(&a.logo_url)
        .bind(&a.screenshots)
        .bind(&a.repo_url)
        .bind(&a.demo_url)
        .bind(&a.website_url)
        .bind(&a.docs_url)
        .bind(&a.technologies)
        .bind(&a.ai_models)
        .bind(Json(&a.agents))
        .bind(&a.human_oversight)
        .bind(Json(&a.evidence))
        .bind(&a.team_name)
        .bind(a.creator_email.as_deref().unwrap_or_default())
        .bind(&a.creator_display_name)
        .bind(a.visibility.as_str())
        .bind(a.status.as_str())
        .bind(rec.source.as_str())
        .bind(&rec.tags_text)
        .bind(a.published_at)
        .bind(a.kind.as_str())
        .bind(&a.model_provider)
        .bind(&a.tools)
        .bind(&a.mcp_servers)
        .bind(&a.capabilities)
        .execute(&mut *tx)
        .await;
        if let Err(sqlx::Error::Database(e)) = &res {
            if e.is_unique_violation() {
                return Err(DomainError::Conflict("that slug is already taken".into()));
            }
        }
        res.map_err(db)?;
        set_links(&mut tx, a.id, &rec.links).await?;
        tx.commit().await.map_err(db)
    }

    /// Content edit. Kind, slug, creator, source, stars and verification never change here.
    pub async fn update(&self, rec: &AppRecord) -> Result<(), DomainError> {
        let a = &rec.app;
        let mut tx = self.pool.begin().await.map_err(db)?;
        sqlx::query(
            r#"
            UPDATE showcase_apps SET
              name = $2, tagline = $3, description_md = $4, description_html = $5,
              app_type = $6, maturity = $7, build_method = $8, pricing = $9, license = $10,
              version = $11, logo_url = $12, screenshots = $13, repo_url = $14, demo_url = $15,
              website_url = $16, docs_url = $17, technologies = $18, ai_models = $19,
              agents = $20, human_oversight = $21, evidence = $22, team_name = $23,
              visibility = $24, status = $25, tags_text = $26, published_at = $27,
              model_provider = $28, tools = $29, mcp_servers = $30, capabilities = $31,
              featured = CASE WHEN $25 = 'published' THEN featured ELSE FALSE END,
              review_note = CASE WHEN $25 = 'pending_review' THEN '' ELSE review_note END,
              updated_at = NOW()
            WHERE id = $1
            "#,
        )
        .bind(a.id)
        .bind(&a.name)
        .bind(&a.tagline)
        .bind(&a.description_md)
        .bind(&a.description_html)
        .bind(a.app_type.as_str())
        .bind(a.maturity.as_str())
        .bind(a.build_method.as_str())
        .bind(a.pricing.as_str())
        .bind(&a.license)
        .bind(&a.version)
        .bind(&a.logo_url)
        .bind(&a.screenshots)
        .bind(&a.repo_url)
        .bind(&a.demo_url)
        .bind(&a.website_url)
        .bind(&a.docs_url)
        .bind(&a.technologies)
        .bind(&a.ai_models)
        .bind(Json(&a.agents))
        .bind(&a.human_oversight)
        .bind(Json(&a.evidence))
        .bind(&a.team_name)
        .bind(a.visibility.as_str())
        .bind(a.status.as_str())
        .bind(&rec.tags_text)
        .bind(a.published_at)
        .bind(&a.model_provider)
        .bind(&a.tools)
        .bind(&a.mcp_servers)
        .bind(&a.capabilities)
        .execute(&mut *tx)
        .await
        .map_err(db)?;
        set_links(&mut tx, a.id, &rec.links).await?;
        tx.commit().await.map_err(db)
    }

    /// Replace an entry's links outside a content edit (sample seeding).
    pub async fn replace_links(
        &self,
        from: ShowcaseAppId,
        links: &[(ShowcaseAppId, String)],
    ) -> Result<(), DomainError> {
        let mut tx = self.pool.begin().await.map_err(db)?;
        set_links(&mut tx, from, links).await?;
        tx.commit().await.map_err(db)
    }

    pub async fn set_moderation(
        &self,
        id: ShowcaseAppId,
        status: ShowcaseStatus,
        featured: bool,
        review_note: &str,
        published_at: Option<DateTime<Utc>>,
    ) -> Result<(), DomainError> {
        sqlx::query(
            r#"
            UPDATE showcase_apps
            SET status = $2, featured = $3, review_note = $4, published_at = $5, updated_at = NOW()
            WHERE id = $1
            "#,
        )
        .bind(id)
        .bind(status.as_str())
        .bind(featured)
        .bind(review_note)
        .bind(published_at)
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(())
    }

    pub async fn delete(&self, id: ShowcaseAppId) -> Result<(), DomainError> {
        sqlx::query("DELETE FROM showcase_apps WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await
            .map_err(db)?;
        Ok(())
    }

    /// Add or remove a star and return the new count. The count is recomputed
    /// in the same statement, so concurrent stars cannot drift it.
    pub async fn set_star(
        &self,
        id: ShowcaseAppId,
        email: &str,
        on: bool,
    ) -> Result<i32, DomainError> {
        let mut tx = self.pool.begin().await.map_err(db)?;
        let change = if on {
            "INSERT INTO showcase_stars (app_id, user_email) VALUES ($1, lower($2)) ON CONFLICT DO NOTHING"
        } else {
            "DELETE FROM showcase_stars WHERE app_id = $1 AND user_email = lower($2)"
        };
        sqlx::query(change)
            .bind(id)
            .bind(email)
            .execute(&mut *tx)
            .await
            .map_err(db)?;
        let count: i32 = sqlx::query_scalar(
            r#"
            UPDATE showcase_apps
            SET star_count = (SELECT COUNT(*) FROM showcase_stars WHERE app_id = $1)
            WHERE id = $1
            RETURNING star_count
            "#,
        )
        .bind(id)
        .fetch_one(&mut *tx)
        .await
        .map_err(db)?;
        tx.commit().await.map_err(db)?;
        Ok(count)
    }
}

async fn set_links(
    tx: &mut Transaction<'_, Postgres>,
    from: ShowcaseAppId,
    links: &[(ShowcaseAppId, String)],
) -> Result<(), DomainError> {
    sqlx::query("DELETE FROM showcase_links WHERE from_id = $1")
        .bind(from)
        .execute(&mut **tx)
        .await
        .map_err(db)?;
    for (position, (to, role)) in links.iter().enumerate() {
        sqlx::query(
            "INSERT INTO showcase_links (from_id, to_id, role, position) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING",
        )
        .bind(from)
        .bind(to)
        .bind(role)
        .bind(position as i32)
        .execute(&mut **tx)
        .await
        .map_err(db)?;
    }
    Ok(())
}

/// `starred` for list queries, where the viewer is bind parameter 12.
const STARRED: &str = "($12::text IS NOT NULL AND EXISTS (
    SELECT 1 FROM showcase_stars s WHERE s.app_id = a.id AND s.user_email = lower($12)))";
/// The same for single-entry lookups, where the viewer is parameter 1.
const STARRED_1: &str = "($1::text IS NOT NULL AND EXISTS (
    SELECT 1 FROM showcase_stars s WHERE s.app_id = a.id AND s.user_email = lower($1)))";

fn from_row(r: &sqlx::postgres::PgRow) -> Result<ShowcaseApp, DomainError> {
    let bad = |what: &str| DomainError::Other(anyhow::anyhow!("invalid {what} in showcase_apps"));
    let text = |col: &str| r.get::<&str, _>(col);
    Ok(ShowcaseApp {
        id: r.get("id"),
        kind: ShowcaseKind::parse(text("kind")).ok_or_else(|| bad("kind"))?,
        slug: r.get("slug"),
        name: r.get("name"),
        tagline: r.get("tagline"),
        description_md: r.get("description_md"),
        description_html: r.get("description_html"),
        app_type: ShowcaseAppType::parse(text("app_type")).ok_or_else(|| bad("app_type"))?,
        maturity: ShowcaseMaturity::parse(text("maturity")).ok_or_else(|| bad("maturity"))?,
        build_method: ShowcaseBuildMethod::parse(text("build_method"))
            .ok_or_else(|| bad("build_method"))?,
        pricing: ShowcasePricing::parse(text("pricing")).ok_or_else(|| bad("pricing"))?,
        license: r.get("license"),
        version: r.get("version"),
        logo_url: r.get("logo_url"),
        screenshots: r.get("screenshots"),
        repo_url: r.get("repo_url"),
        demo_url: r.get("demo_url"),
        website_url: r.get("website_url"),
        docs_url: r.get("docs_url"),
        technologies: r.get("technologies"),
        ai_models: r.get("ai_models"),
        // Stored by this code, but a hand-edited row should not take the page down.
        agents: r
            .try_get::<Json<_>, _>("agents")
            .map(|j| j.0)
            .unwrap_or_default(),
        human_oversight: r.get("human_oversight"),
        evidence: r
            .try_get::<Json<_>, _>("evidence")
            .map(|j| j.0)
            .unwrap_or_default(),
        team_name: r.get("team_name"),
        model_provider: r.get("model_provider"),
        tools: r.get("tools"),
        mcp_servers: r.get("mcp_servers"),
        capabilities: r.get("capabilities"),
        linked_agents: Vec::new(),
        linked_team: None,
        used_in: r.get("used_in"),
        creator_email: Some(r.get("creator_email")),
        creator_display_name: r.get("creator_display_name"),
        visibility: ShowcaseVisibility::parse(text("visibility"))
            .ok_or_else(|| bad("visibility"))?,
        status: ShowcaseStatus::parse(text("status")).ok_or_else(|| bad("status"))?,
        verification: ShowcaseVerification::parse(text("verification"))
            .ok_or_else(|| bad("verification"))?,
        featured: r.get("featured"),
        review_note: r.get("review_note"),
        star_count: r.get("star_count"),
        starred: r.get("starred"),
        published_at: r.get("published_at"),
        created_at: r.get("created_at"),
        updated_at: r.get("updated_at"),
    })
}
