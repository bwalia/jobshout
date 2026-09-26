//! Insights hub: posts, articles, blogs, podcasts, videos and the newsletter.
//!
//! Business rules live in [`rules`] (pure, unit-tested); this service wires
//! them to the repository.

#![forbid(unsafe_code)]

pub mod feed;
pub mod media;
pub mod render;
mod repo;
pub mod rules;
mod seed;

use std::collections::HashSet;
use std::sync::Arc;

use chrono::{Duration, Utc};
use jobshout_domain::{
    DomainError, Insight, InsightId, InsightInput, InsightKind, InsightStatus, InsightTopic,
    NewsletterDigest, NewsletterStatus,
};
use sqlx::PgPool;
use uuid::Uuid;

pub use repo::ListQuery;
use repo::{InsightRepository, ItemRecord};
pub use rules::{Actor, Moderation};

/// The synthetic address an agent's submissions are filed under.
fn agent_email(name: &str) -> String {
    format!("{}@agents.jobshout.com", rules::slugify(name.trim()))
}

#[derive(Clone)]
pub struct InsightService {
    repo: InsightRepository,
    staff: Arc<HashSet<String>>,
    /// Agent addresses whose submissions publish directly (see
    /// `with_trusted_agents`).
    trusted_agents: Arc<HashSet<String>>,
}

impl InsightService {
    /// `staff_emails` may publish without review and moderate the queue.
    /// Until the marketplace has real roles this allowlist is the whole of it.
    pub fn new(pool: PgPool, staff_emails: impl IntoIterator<Item = String>) -> Self {
        Self {
            repo: InsightRepository::new(pool),
            staff: Arc::new(
                staff_emails
                    .into_iter()
                    .map(|e| e.trim().to_ascii_lowercase())
                    .filter(|e| !e.is_empty())
                    .collect(),
            ),
            trusted_agents: Arc::new(HashSet::new()),
        }
    }

    /// Agents, by the name they submit as (e.g. "JobShout.com Content Writer"),
    /// whose submitted items are published without review.
    ///
    /// Only for an agent whose platform already requires a person to approve
    /// the item before sending it — the Content Writer sends an article only
    /// when someone publishes it live — so the review queue would be a second
    /// approval of the same decision. Every other agent still goes to review.
    pub fn with_trusted_agents(mut self, names: impl IntoIterator<Item = String>) -> Self {
        self.trusted_agents = Arc::new(
            names
                .into_iter()
                .map(|n| n.trim().to_string())
                .filter(|n| !n.is_empty())
                .map(|n| agent_email(&n))
                .collect(),
        );
        self
    }

    fn trusts(&self, actor: &Actor) -> bool {
        actor.agent && self.trusted_agents.contains(&actor.email)
    }

    pub fn actor(&self, email: &str, name: &str) -> Actor {
        let email = email.trim().to_string();
        Actor {
            is_staff: self.staff.contains(&email.to_ascii_lowercase()),
            name: name.trim().to_string(),
            email,
            agent: false,
        }
    }

    /// An automated author, named by the caller (e.g. "Article Writer"). It
    /// gets a stable synthetic address so its submissions group under one
    /// author, and it can never be staff whatever the allowlist says.
    pub fn agent_actor(&self, name: &str) -> Result<Actor, DomainError> {
        let name = name.trim();
        if name.is_empty() || name.chars().count() > 60 {
            return Err(DomainError::Validation(
                "agent name must be 1-60 characters".into(),
            ));
        }
        Ok(Actor {
            email: agent_email(name),
            name: format!("JobShout {name}"),
            is_staff: false,
            agent: true,
        })
    }

    pub async fn topics(&self) -> Result<Vec<InsightTopic>, DomainError> {
        self.repo.topics().await
    }

    /// The public feed: published items only, author emails removed.
    pub async fn list_published(
        &self,
        mut q: ListQuery,
    ) -> Result<(Vec<Insight>, i64), DomainError> {
        q.status = Some(InsightStatus::Published);
        q.author_email = None;
        q.limit = q.limit.clamp(1, 50);
        q.offset = q.offset.max(0);
        q.q = q.q.filter(|s| !s.trim().is_empty());
        let (items, total) = self.repo.list(&q).await?;
        Ok((items.into_iter().map(public).collect(), total))
    }

    pub async fn get_by_slug(
        &self,
        slug: &str,
        actor: Option<&Actor>,
    ) -> Result<Insight, DomainError> {
        let item = self.repo.get_by_slug(slug).await?;
        if !rules::can_view(actor, &item) {
            // Hidden items look missing rather than forbidden.
            return Err(DomainError::NotFound);
        }
        Ok(if actor.is_some_and(|a| a.is_staff || a.owns(&item)) {
            item
        } else {
            public(item)
        })
    }

    pub async fn related(&self, slug: &str, limit: i64) -> Result<Vec<Insight>, DomainError> {
        let item = self.repo.get_by_slug(slug).await?;
        let mut q = ListQuery {
            topic: item.topics.first().map(|t| t.slug.clone()),
            exclude: Some(item.id),
            limit,
            ..Default::default()
        };
        let (mut items, _) = self.list_published(q.clone()).await?;
        if items.len() < limit as usize {
            // Topic too thin: top up with the latest of anything.
            q.topic = None;
            let (more, _) = self.list_published(q).await?;
            let seen: HashSet<_> = items.iter().map(|i| i.id).collect();
            items.extend(more.into_iter().filter(|i| !seen.contains(&i.id)));
            items.truncate(limit as usize);
        }
        Ok(items)
    }

    pub async fn list_mine(&self, actor: &Actor) -> Result<Vec<Insight>, DomainError> {
        let (items, _) = self
            .repo
            .list(&ListQuery {
                author_email: Some(actor.email.clone()),
                limit: 100,
                ..Default::default()
            })
            .await?;
        Ok(items)
    }

    pub async fn review_queue(&self, actor: &Actor) -> Result<Vec<Insight>, DomainError> {
        require_staff(actor)?;
        let (items, _) = self
            .repo
            .list(&ListQuery {
                status: Some(InsightStatus::PendingReview),
                limit: 100,
                ..Default::default()
            })
            .await?;
        Ok(items)
    }

    pub async fn create(&self, actor: &Actor, input: InsightInput) -> Result<Insight, DomainError> {
        if !actor.is_staff && !actor.agent {
            let recent = self
                .repo
                .created_since(&actor.email, Utc::now() - Duration::hours(1))
                .await?;
            if recent >= rules::CREATES_PER_HOUR {
                return Err(DomainError::RateLimited(
                    "that is a lot of new insights in an hour — try again shortly".into(),
                ));
            }
        }
        let kind = input
            .kind
            .ok_or_else(|| DomainError::Validation("kind is required".into()))?;
        let status = rules::status_on_create(actor.source(), input.submit, self.trusts(actor));
        let base = rules::slugify(&input.title);
        let slug = rules::next_free_slug(&base, &self.repo.slugs_like(&base).await?);
        let mut rec = self.record(kind, &input, status).await?;
        rec.id = Uuid::new_v4();
        rec.slug = slug;
        rec.author_email = actor.email.clone();
        rec.author_display_name = if actor.name.is_empty() {
            actor
                .email
                .split('@')
                .next()
                .unwrap_or_default()
                .to_string()
        } else {
            actor.name.clone()
        };
        rec.source = actor.source();
        self.repo.insert(&rec).await?;
        self.repo.get(rec.id).await
    }

    pub async fn update(
        &self,
        actor: &Actor,
        id: InsightId,
        input: InsightInput,
    ) -> Result<Insight, DomainError> {
        let existing = self.repo.get(id).await?;
        rules::check_can_edit(actor, &existing)?;
        let kind = input.kind.unwrap_or(existing.kind);
        // Staff editing a live item keep it live; everyone else re-enters the
        // same flow as a new item.
        let status = if actor.is_staff && existing.status == InsightStatus::Published {
            InsightStatus::Published
        } else {
            rules::status_on_save(actor.source(), input.submit)
        };
        let mut rec = self.record(kind, &input, status).await?;
        rec.id = existing.id;
        rec.published_at = match status {
            InsightStatus::Published => existing.published_at.or(Some(Utc::now())),
            _ => None,
        };
        self.repo.update(&rec).await?;
        self.repo.get(id).await
    }

    pub async fn delete(&self, actor: &Actor, id: InsightId) -> Result<(), DomainError> {
        let existing = self.repo.get(id).await?;
        rules::check_can_edit(actor, &existing)?;
        self.repo.delete(id).await
    }

    pub async fn moderate(
        &self,
        actor: &Actor,
        id: InsightId,
        action: Moderation,
    ) -> Result<Insight, DomainError> {
        require_staff(actor)?;
        let item = self.repo.get(id).await?;
        let status = rules::moderate(item.status, &action)?;
        let featured = match action {
            Moderation::Feature(on) => on,
            _ if status == InsightStatus::Published => item.featured,
            _ => false,
        };
        let note = match &action {
            Moderation::Reject { note } => note.trim().to_string(),
            _ => String::new(),
        };
        let published_at = match status {
            InsightStatus::Published => item.published_at.or(Some(Utc::now())),
            _ => None,
        };
        self.repo
            .set_moderation(id, status, featured, &note, published_at)
            .await?;
        self.repo.get(id).await
    }

    pub fn preview(&self, body_md: &str) -> String {
        render::markdown_to_html(body_md)
    }

    /// Weekly digests of what was published, newest week first.
    pub async fn digests(&self, weeks: i64) -> Result<Vec<NewsletterDigest>, DomainError> {
        let (items, _) = self
            .list_published(ListQuery {
                limit: 50,
                ..Default::default()
            })
            .await?;
        let since = Utc::now() - Duration::weeks(weeks.clamp(1, 52));
        Ok(rules::group_by_week(
            items
                .into_iter()
                .filter(|i| i.published_at.is_some_and(|p| p >= since))
                .collect(),
        ))
    }

    /// Everything the feeds need: published items, newest first.
    pub async fn feed_items(&self, kind: Option<InsightKind>) -> Result<Vec<Insight>, DomainError> {
        let (items, _) = self
            .list_published(ListQuery {
                kind,
                limit: 50,
                ..Default::default()
            })
            .await?;
        Ok(items)
    }

    /// Returns a confirmation token, or None when the address is already on
    /// the list. The caller decides how the token reaches the subscriber.
    pub async fn subscribe(&self, email: &str) -> Result<Option<String>, DomainError> {
        if !rules::is_email(email) {
            return Err(DomainError::Validation(
                "that does not look like an email address".into(),
            ));
        }
        self.repo.upsert_subscriber(email).await
    }

    pub async fn confirm(&self, token: &str) -> Result<String, DomainError> {
        self.repo
            .set_subscriber_status(token, NewsletterStatus::Confirmed)
            .await
    }

    pub async fn unsubscribe(&self, token: &str) -> Result<String, DomainError> {
        self.repo
            .set_subscriber_status(token, NewsletterStatus::Unsubscribed)
            .await
    }

    /// Dev/int only: fill an empty hub with clearly-labelled sample items.
    pub async fn seed_samples(&self) -> Result<usize, DomainError> {
        if self.repo.count().await? > 0 {
            return Ok(0);
        }
        let editor = Actor {
            email: "editors@jobshout.com".into(),
            name: "JobShout Editors".into(),
            is_staff: true,
            agent: false,
        };
        let samples = seed::samples();
        let n = samples.len();
        for (input, featured) in samples {
            let item = self.create(&editor, input).await?;
            if featured {
                self.moderate(&editor, item.id, Moderation::Feature(true))
                    .await?;
            }
        }
        Ok(n)
    }

    async fn record(
        &self,
        kind: InsightKind,
        input: &InsightInput,
        status: InsightStatus,
    ) -> Result<ItemRecord, DomainError> {
        let known: HashSet<String> = self
            .repo
            .topics()
            .await?
            .into_iter()
            .map(|t| t.slug)
            .collect();
        let media = rules::validate(kind, input, &known)?;
        let body_md = input.body_md.trim().to_string();
        Ok(ItemRecord {
            id: Uuid::nil(),
            kind,
            slug: String::new(),
            title: input.title.trim().to_string(),
            summary: input.summary.trim().to_string(),
            body_html: render::markdown_to_html(&body_md),
            reading_minutes: render::reading_minutes(&body_md),
            body_md,
            cover_image_url: input.cover_image_url.trim().to_string(),
            cover_image_alt: input.cover_image_alt.trim().to_string(),
            link_url: input.link_url.trim().to_string(),
            media_url: input.media_url.trim().to_string(),
            embed_provider: media.provider,
            embed_url: media.embed_url,
            duration_seconds: input.duration_seconds,
            transcript: input.transcript.trim().to_string(),
            author_email: String::new(),
            author_display_name: String::new(),
            status,
            source: jobshout_domain::InsightSource::Community,
            published_at: (status == InsightStatus::Published).then(Utc::now),
            topics: input.topics.clone(),
        })
    }
}

fn require_staff(actor: &Actor) -> Result<(), DomainError> {
    if actor.is_staff {
        Ok(())
    } else {
        Err(DomainError::Forbidden("only editors can do that".into()))
    }
}

/// Strip what the public should not see.
fn public(mut item: Insight) -> Insight {
    item.author_email = None;
    item.review_note = String::new();
    item
}
