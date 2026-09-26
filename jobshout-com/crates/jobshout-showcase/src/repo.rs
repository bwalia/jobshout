use std::collections::HashSet;

use chrono::{DateTime, Utc};
use jobshout_domain::{
    DomainError, InsightSource, ShowcaseApp, ShowcaseAppId, ShowcaseAppType, ShowcaseBuildMethod,
    ShowcaseMaturity, ShowcasePricing, ShowcaseStatus, ShowcaseTag, ShowcaseVerification,
    ShowcaseVisibility,
};
use sqlx::types::Json;
use sqlx::{PgPool, Row};

const COLUMNS: &str = r#"
    a.id, a.slug, a.name, a.tagline, a.description_md, a.description_html, a.app_type,
    a.maturity, a.build_method, a.pricing, a.license, a.version, a.logo_url, a.screenshots,
    a.repo_url, a.demo_url, a.website_url, a.docs_url, a.technologies, a.ai_models, a.agents,
    a.human_oversight, a.evidence, a.team_name, a.creator_email, a.creator_display_name,
    a.visibility, a.status, a.verification, a.featured, a.review_note, a.star_count,
    a.published_at, a.created_at, a.updated_at
"#;

fn db(e: sqlx::Error) -> DomainError {
    DomainError::Other(e.into())
}

/// Everything a write stores. Built by the service after validation.
pub struct AppRecord {
    pub app: ShowcaseApp,
    pub source: InsightSource,
    pub tags_text: String,
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
    pub status: Option<ShowcaseStatus>,
    /// Lists and search only ever show public apps; unlisted ones are reachable by link.
    pub public_only: bool,
    pub creator_email: Option<String>,
    pub q: Option<String>,
    pub app_type: Option<ShowcaseAppType>,
    pub maturities: Vec<ShowcaseMaturity>,
    pub build_methods: Vec<ShowcaseBuildMethod>,
    pub pricing: Option<ShowcasePricing>,
    pub technology: Option<String>,
    pub featured: Option<bool>,
    pub exclude: Option<ShowcaseAppId>,
    /// Marks `starred` on each result for this viewer.
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
            SELECT {COLUMNS}, {STARRED} AS starred, COUNT(*) OVER() AS total
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
            .fetch_all(&self.pool)
            .await
            .map_err(db)?;
        let total = rows.first().map(|r| r.get::<i64, _>("total")).unwrap_or(0);
        let apps = rows.iter().map(from_row).collect::<Result<Vec<_>, _>>()?;
        Ok((apps, total))
    }

    pub async fn get_by_slug(
        &self,
        slug: &str,
        viewer_email: Option<&str>,
    ) -> Result<ShowcaseApp, DomainError> {
        let sql = format!(
            "SELECT {COLUMNS}, {STARRED_1} AS starred FROM showcase_apps a WHERE a.slug = $2"
        );
        self.fetch_one(&sql, viewer_email, slug.to_string()).await
    }

    pub async fn get(&self, id: ShowcaseAppId) -> Result<ShowcaseApp, DomainError> {
        let sql = format!(
            "SELECT {COLUMNS}, {STARRED_1} AS starred FROM showcase_apps a WHERE a.id = $2"
        );
        self.fetch_one(&sql, None, id).await
    }

    async fn fetch_one<T>(
        &self,
        sql: &str,
        viewer_email: Option<&str>,
        key: T,
    ) -> Result<ShowcaseApp, DomainError>
    where
        T: for<'q> sqlx::Encode<'q, sqlx::Postgres> + sqlx::Type<sqlx::Postgres> + Send,
    {
        let row = sqlx::query(sql)
            .bind(viewer_email)
            .bind(key)
            .fetch_optional(&self.pool)
            .await
            .map_err(db)?
            .ok_or(DomainError::NotFound)?;
        from_row(&row)
    }

    /// Technologies across public, published apps, most used first.
    pub async fn tags(&self, limit: i64) -> Result<Vec<ShowcaseTag>, DomainError> {
        let rows = sqlx::query(
            r#"
            SELECT min(t) AS name, COUNT(*) AS count
            FROM showcase_apps a, unnest(a.technologies) t
            WHERE a.status = 'published' AND a.visibility = 'public'
            GROUP BY lower(t)
            ORDER BY count DESC, name
            LIMIT $1
            "#,
        )
        .bind(limit)
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

    pub async fn count(&self) -> Result<i64, DomainError> {
        sqlx::query_scalar("SELECT COUNT(*) FROM showcase_apps")
            .fetch_one(&self.pool)
            .await
            .map_err(db)
    }

    pub async fn insert(&self, rec: &AppRecord) -> Result<(), DomainError> {
        let a = &rec.app;
        let res = sqlx::query(
            r#"
            INSERT INTO showcase_apps (
              id, slug, name, tagline, description_md, description_html, app_type, maturity,
              build_method, pricing, license, version, logo_url, screenshots, repo_url, demo_url,
              website_url, docs_url, technologies, ai_models, agents, human_oversight, evidence,
              team_name, creator_email, creator_display_name, visibility, status, source,
              tags_text, published_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
                      $21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31)
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
        .execute(&self.pool)
        .await;
        if let Err(sqlx::Error::Database(e)) = &res {
            if e.is_unique_violation() {
                return Err(DomainError::Conflict("that slug is already taken".into()));
            }
        }
        res.map_err(db)?;
        Ok(())
    }

    /// Content edit. Slug, creator, source, stars and verification never change here.
    pub async fn update(&self, rec: &AppRecord) -> Result<(), DomainError> {
        let a = &rec.app;
        sqlx::query(
            r#"
            UPDATE showcase_apps SET
              name = $2, tagline = $3, description_md = $4, description_html = $5,
              app_type = $6, maturity = $7, build_method = $8, pricing = $9, license = $10,
              version = $11, logo_url = $12, screenshots = $13, repo_url = $14, demo_url = $15,
              website_url = $16, docs_url = $17, technologies = $18, ai_models = $19,
              agents = $20, human_oversight = $21, evidence = $22, team_name = $23,
              visibility = $24, status = $25, tags_text = $26, published_at = $27,
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
        .execute(&self.pool)
        .await
        .map_err(db)?;
        Ok(())
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

/// `starred` for list queries, where the viewer is bind parameter 12.
const STARRED: &str = "($12::text IS NOT NULL AND EXISTS (
    SELECT 1 FROM showcase_stars s WHERE s.app_id = a.id AND s.user_email = lower($12)))";
/// The same for single-app lookups, where the viewer is parameter 1.
const STARRED_1: &str = "($1::text IS NOT NULL AND EXISTS (
    SELECT 1 FROM showcase_stars s WHERE s.app_id = a.id AND s.user_email = lower($1)))";

fn from_row(r: &sqlx::postgres::PgRow) -> Result<ShowcaseApp, DomainError> {
    let bad = |what: &str| DomainError::Other(anyhow::anyhow!("invalid {what} in showcase_apps"));
    let text = |col: &str| r.get::<&str, _>(col);
    Ok(ShowcaseApp {
        id: r.get("id"),
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
