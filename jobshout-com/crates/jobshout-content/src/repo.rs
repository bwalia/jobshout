use std::collections::{HashMap, HashSet};

use chrono::{DateTime, Utc};
use jobshout_domain::{
    DomainError, Insight, InsightId, InsightKind, InsightSource, InsightStatus, InsightTopic,
    NewsletterStatus,
};
use sqlx::{PgPool, Postgres, Row, Transaction};
use uuid::Uuid;

const COLUMNS: &str = r#"
    i.id, i.kind, i.slug, i.title, i.summary, i.body_md, i.body_html,
    i.cover_image_url, i.cover_image_alt, i.link_url, i.media_url, i.embed_provider,
    i.embed_url, i.duration_seconds, i.transcript, i.author_email, i.author_display_name,
    i.status, i.source, i.featured, i.review_note, i.reading_minutes,
    i.published_at, i.created_at, i.updated_at
"#;

fn db(e: sqlx::Error) -> DomainError {
    DomainError::Other(e.into())
}

/// Everything a write stores. Built by the service after validation.
pub struct ItemRecord {
    pub id: InsightId,
    pub kind: InsightKind,
    pub slug: String,
    pub title: String,
    pub summary: String,
    pub body_md: String,
    pub body_html: String,
    pub cover_image_url: String,
    pub cover_image_alt: String,
    pub link_url: String,
    pub media_url: String,
    pub embed_provider: String,
    pub embed_url: String,
    pub duration_seconds: Option<i32>,
    pub transcript: String,
    pub author_email: String,
    pub author_display_name: String,
    pub status: InsightStatus,
    pub source: InsightSource,
    pub reading_minutes: i32,
    pub published_at: Option<DateTime<Utc>>,
    pub topics: Vec<String>,
}

#[derive(Debug, Default, Clone)]
pub struct ListQuery {
    pub status: Option<InsightStatus>,
    pub kind: Option<InsightKind>,
    pub topic: Option<String>,
    pub q: Option<String>,
    pub featured: Option<bool>,
    pub author_email: Option<String>,
    pub exclude: Option<InsightId>,
    pub since: Option<DateTime<Utc>>,
    pub limit: i64,
    pub offset: i64,
}

#[derive(Clone)]
pub struct InsightRepository {
    pool: PgPool,
}

impl InsightRepository {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn topics(&self) -> Result<Vec<InsightTopic>, DomainError> {
        let rows = sqlx::query(
            "SELECT slug, name, description FROM insights_topics ORDER BY position, name",
        )
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        Ok(rows
            .iter()
            .map(|r| InsightTopic {
                slug: r.get("slug"),
                name: r.get("name"),
                description: r.get("description"),
            })
            .collect())
    }

    pub async fn list(&self, q: &ListQuery) -> Result<(Vec<Insight>, i64), DomainError> {
        let sql = format!(
            r#"
            SELECT {COLUMNS}, COUNT(*) OVER() AS total
            FROM insights_items i
            WHERE ($1::text IS NULL OR i.status = $1)
              AND ($2::text IS NULL OR i.kind = $2)
              AND ($3::text IS NULL OR EXISTS (
                    SELECT 1 FROM insights_item_topics t
                    WHERE t.item_id = i.id AND t.topic_slug = $3))
              AND ($4::text IS NULL OR i.search @@ websearch_to_tsquery('english', $4))
              AND ($5::bool IS NULL OR i.featured = $5)
              AND ($6::text IS NULL OR lower(i.author_email) = lower($6))
              AND ($7::uuid IS NULL OR i.id <> $7)
              AND ($8::timestamptz IS NULL OR i.published_at >= $8)
            ORDER BY COALESCE(i.published_at, i.updated_at) DESC, i.created_at DESC
            LIMIT $9 OFFSET $10
            "#
        );
        let rows = sqlx::query(&sql)
            .bind(q.status.map(|s| s.as_str()))
            .bind(q.kind.map(|k| k.as_str()))
            .bind(q.topic.as_deref())
            .bind(q.q.as_deref())
            .bind(q.featured)
            .bind(q.author_email.as_deref())
            .bind(q.exclude)
            .bind(q.since)
            .bind(q.limit)
            .bind(q.offset)
            .fetch_all(&self.pool)
            .await
            .map_err(db)?;
        let total = rows.first().map(|r| r.get::<i64, _>("total")).unwrap_or(0);
        let mut items = rows.iter().map(from_row).collect::<Result<Vec<_>, _>>()?;
        self.attach_topics(&mut items).await?;
        Ok((items, total))
    }

    pub async fn get_by_slug(&self, slug: &str) -> Result<Insight, DomainError> {
        self.get_where("i.slug = $1", slug.to_string()).await
    }

    pub async fn get(&self, id: InsightId) -> Result<Insight, DomainError> {
        self.get_where("i.id = $1", id).await
    }

    async fn get_where<T>(&self, clause: &str, value: T) -> Result<Insight, DomainError>
    where
        T: for<'q> sqlx::Encode<'q, Postgres> + sqlx::Type<Postgres> + Send,
    {
        let sql = format!("SELECT {COLUMNS} FROM insights_items i WHERE {clause}");
        let row = sqlx::query(&sql)
            .bind(value)
            .fetch_optional(&self.pool)
            .await
            .map_err(db)?
            .ok_or(DomainError::NotFound)?;
        let mut items = vec![from_row(&row)?];
        self.attach_topics(&mut items).await?;
        Ok(items.remove(0))
    }

    async fn attach_topics(&self, items: &mut [Insight]) -> Result<(), DomainError> {
        if items.is_empty() {
            return Ok(());
        }
        let ids: Vec<Uuid> = items.iter().map(|i| i.id).collect();
        let rows = sqlx::query(
            r#"
            SELECT it.item_id, t.slug, t.name, t.description
            FROM insights_item_topics it
            JOIN insights_topics t ON t.slug = it.topic_slug
            WHERE it.item_id = ANY($1)
            ORDER BY t.position
            "#,
        )
        .bind(&ids)
        .fetch_all(&self.pool)
        .await
        .map_err(db)?;
        let mut by_item: HashMap<Uuid, Vec<InsightTopic>> = HashMap::new();
        for r in &rows {
            by_item
                .entry(r.get("item_id"))
                .or_default()
                .push(InsightTopic {
                    slug: r.get("slug"),
                    name: r.get("name"),
                    description: r.get("description"),
                });
        }
        for item in items {
            item.topics = by_item.remove(&item.id).unwrap_or_default();
        }
        Ok(())
    }

    /// Slugs already taken that start with `base`, for picking a free one.
    pub async fn slugs_like(&self, base: &str) -> Result<HashSet<String>, DomainError> {
        let rows = sqlx::query("SELECT slug FROM insights_items WHERE slug = $1 OR slug LIKE $2")
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
            "SELECT COUNT(*) FROM insights_items WHERE lower(author_email) = lower($1) AND created_at >= $2",
        )
        .bind(email)
        .bind(since)
        .fetch_one(&self.pool)
        .await
        .map_err(db)
    }

    pub async fn count(&self) -> Result<i64, DomainError> {
        sqlx::query_scalar("SELECT COUNT(*) FROM insights_items")
            .fetch_one(&self.pool)
            .await
            .map_err(db)
    }

    pub async fn insert(&self, rec: &ItemRecord) -> Result<(), DomainError> {
        let mut tx = self.pool.begin().await.map_err(db)?;
        let res = sqlx::query(
            r#"
            INSERT INTO insights_items (
              id, kind, slug, title, summary, body_md, body_html, cover_image_url,
              cover_image_alt, link_url, media_url, embed_provider, embed_url,
              duration_seconds, transcript, author_email, author_display_name, status,
              source, reading_minutes, published_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
            "#,
        )
        .bind(rec.id)
        .bind(rec.kind.as_str())
        .bind(&rec.slug)
        .bind(&rec.title)
        .bind(&rec.summary)
        .bind(&rec.body_md)
        .bind(&rec.body_html)
        .bind(&rec.cover_image_url)
        .bind(&rec.cover_image_alt)
        .bind(&rec.link_url)
        .bind(&rec.media_url)
        .bind(&rec.embed_provider)
        .bind(&rec.embed_url)
        .bind(rec.duration_seconds)
        .bind(&rec.transcript)
        .bind(&rec.author_email)
        .bind(&rec.author_display_name)
        .bind(rec.status.as_str())
        .bind(rec.source.as_str())
        .bind(rec.reading_minutes)
        .bind(rec.published_at)
        .execute(&mut *tx)
        .await;
        if let Err(sqlx::Error::Database(e)) = &res {
            if e.is_unique_violation() {
                return Err(DomainError::Conflict("that slug is already taken".into()));
            }
        }
        res.map_err(db)?;
        set_topics(&mut tx, rec.id, &rec.topics).await?;
        tx.commit().await.map_err(db)
    }

    /// Content edit. Slug, author and source never change after creation.
    pub async fn update(&self, rec: &ItemRecord) -> Result<(), DomainError> {
        let mut tx = self.pool.begin().await.map_err(db)?;
        sqlx::query(
            r#"
            UPDATE insights_items SET
              kind = $2, title = $3, summary = $4, body_md = $5, body_html = $6,
              cover_image_url = $7, cover_image_alt = $8, link_url = $9, media_url = $10,
              embed_provider = $11, embed_url = $12, duration_seconds = $13, transcript = $14,
              status = $15, reading_minutes = $16, published_at = $17,
              review_note = CASE WHEN $15 = 'pending_review' THEN '' ELSE review_note END,
              updated_at = NOW()
            WHERE id = $1
            "#,
        )
        .bind(rec.id)
        .bind(rec.kind.as_str())
        .bind(&rec.title)
        .bind(&rec.summary)
        .bind(&rec.body_md)
        .bind(&rec.body_html)
        .bind(&rec.cover_image_url)
        .bind(&rec.cover_image_alt)
        .bind(&rec.link_url)
        .bind(&rec.media_url)
        .bind(&rec.embed_provider)
        .bind(&rec.embed_url)
        .bind(rec.duration_seconds)
        .bind(&rec.transcript)
        .bind(rec.status.as_str())
        .bind(rec.reading_minutes)
        .bind(rec.published_at)
        .execute(&mut *tx)
        .await
        .map_err(db)?;
        set_topics(&mut tx, rec.id, &rec.topics).await?;
        tx.commit().await.map_err(db)
    }

    pub async fn set_moderation(
        &self,
        id: InsightId,
        status: InsightStatus,
        featured: bool,
        review_note: &str,
        published_at: Option<DateTime<Utc>>,
    ) -> Result<(), DomainError> {
        sqlx::query(
            r#"
            UPDATE insights_items
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

    pub async fn delete(&self, id: InsightId) -> Result<(), DomainError> {
        sqlx::query("DELETE FROM insights_items WHERE id = $1")
            .bind(id)
            .execute(&self.pool)
            .await
            .map_err(db)?;
        Ok(())
    }

    /* --- Newsletter -------------------------------------------------------- */

    /// Returns the token to confirm with, or None when already confirmed.
    pub async fn upsert_subscriber(&self, email: &str) -> Result<Option<String>, DomainError> {
        let token = format!("{}{}", Uuid::new_v4().simple(), Uuid::new_v4().simple());
        // A pending or unsubscribed address gets a fresh token and starts over;
        // a confirmed one is left alone and nothing is sent.
        let row = sqlx::query(
            r#"
            INSERT INTO insights_newsletter_subscribers (id, email, status, token)
            VALUES ($1, lower($2), 'pending', $3)
            ON CONFLICT (lower(email)) DO UPDATE
              SET token = EXCLUDED.token, status = 'pending', unsubscribed_at = NULL
              WHERE insights_newsletter_subscribers.status <> 'confirmed'
            RETURNING token
            "#,
        )
        .bind(Uuid::new_v4())
        .bind(email.trim())
        .bind(&token)
        .fetch_optional(&self.pool)
        .await
        .map_err(db)?;
        Ok(row.map(|r| r.get("token")))
    }

    pub async fn set_subscriber_status(
        &self,
        token: &str,
        status: NewsletterStatus,
    ) -> Result<String, DomainError> {
        let row = sqlx::query(
            r#"
            UPDATE insights_newsletter_subscribers SET
              status = $2,
              confirmed_at = CASE WHEN $2 = 'confirmed' THEN NOW() ELSE confirmed_at END,
              unsubscribed_at = CASE WHEN $2 = 'unsubscribed' THEN NOW() ELSE NULL END
            WHERE token = $1
            RETURNING email
            "#,
        )
        .bind(token)
        .bind(status.as_str())
        .fetch_optional(&self.pool)
        .await
        .map_err(db)?
        .ok_or(DomainError::NotFound)?;
        Ok(row.get("email"))
    }
}

async fn set_topics(
    tx: &mut Transaction<'_, Postgres>,
    id: InsightId,
    topics: &[String],
) -> Result<(), DomainError> {
    sqlx::query("DELETE FROM insights_item_topics WHERE item_id = $1")
        .bind(id)
        .execute(&mut **tx)
        .await
        .map_err(db)?;
    if !topics.is_empty() {
        sqlx::query(
            "INSERT INTO insights_item_topics (item_id, topic_slug) SELECT $1, unnest($2::text[]) ON CONFLICT DO NOTHING",
        )
        .bind(id)
        .bind(topics)
        .execute(&mut **tx)
        .await
        .map_err(db)?;
    }
    Ok(())
}

fn from_row(r: &sqlx::postgres::PgRow) -> Result<Insight, DomainError> {
    let bad = |what: &str| DomainError::Other(anyhow::anyhow!("invalid {what} in insights_items"));
    Ok(Insight {
        id: r.get("id"),
        kind: InsightKind::parse(r.get::<&str, _>("kind")).ok_or_else(|| bad("kind"))?,
        slug: r.get("slug"),
        title: r.get("title"),
        summary: r.get("summary"),
        body_md: r.get("body_md"),
        body_html: r.get("body_html"),
        cover_image_url: r.get("cover_image_url"),
        cover_image_alt: r.get("cover_image_alt"),
        link_url: r.get("link_url"),
        media_url: r.get("media_url"),
        embed_provider: r.get("embed_provider"),
        embed_url: r.get("embed_url"),
        duration_seconds: r.get("duration_seconds"),
        transcript: r.get("transcript"),
        author_email: Some(r.get("author_email")),
        author_display_name: r.get("author_display_name"),
        status: InsightStatus::parse(r.get::<&str, _>("status")).ok_or_else(|| bad("status"))?,
        source: InsightSource::parse(r.get::<&str, _>("source")).ok_or_else(|| bad("source"))?,
        featured: r.get("featured"),
        review_note: r.get("review_note"),
        reading_minutes: r.get("reading_minutes"),
        topics: Vec::new(),
        published_at: r.get("published_at"),
        created_at: r.get("created_at"),
        updated_at: r.get("updated_at"),
    })
}
