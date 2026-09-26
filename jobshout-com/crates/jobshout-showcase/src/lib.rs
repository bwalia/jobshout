//! AI Showcase: applications built by people and AI agents.
//!
//! Business rules live in [`rules`] (pure, unit-tested); this service wires
//! them to the repository. Identity and the editors list are shared with
//! Insights: an Insights editor moderates the showcase too.

#![forbid(unsafe_code)]

mod repo;
pub mod rules;
mod seed;

use chrono::{Duration, Utc};
use jobshout_content::render::markdown_to_html;
use jobshout_content::rules::{next_free_slug, slugify};
use jobshout_content::Actor;
use jobshout_domain::{
    DomainError, ShowcaseApp, ShowcaseAppId, ShowcaseAppInput, ShowcaseAppType,
    ShowcaseBuildMethod, ShowcaseMaturity, ShowcasePricing, ShowcaseStatus, ShowcaseTag,
    ShowcaseVerification, ShowcaseVisibility,
};
use sqlx::PgPool;
use uuid::Uuid;

use repo::{AppRecord, ShowcaseRepository};
pub use repo::{ListQuery, Sort};
pub use rules::Moderation;

#[derive(Clone)]
pub struct ShowcaseService {
    repo: ShowcaseRepository,
}

impl ShowcaseService {
    pub fn new(pool: PgPool) -> Self {
        Self {
            repo: ShowcaseRepository::new(pool),
        }
    }

    /// The public list: published, public apps only, creator emails removed.
    pub async fn list_published(
        &self,
        mut q: ListQuery,
    ) -> Result<(Vec<ShowcaseApp>, i64), DomainError> {
        q.status = Some(ShowcaseStatus::Published);
        q.public_only = true;
        q.creator_email = None;
        q.limit = q.limit.clamp(1, 50);
        q.offset = q.offset.max(0);
        q.q = q.q.filter(|s| !s.trim().is_empty());
        q.technology = q.technology.filter(|s| !s.trim().is_empty());
        let (apps, total) = self.repo.list(&q).await?;
        Ok((apps.into_iter().map(public).collect(), total))
    }

    pub async fn tags(&self, limit: i64) -> Result<Vec<ShowcaseTag>, DomainError> {
        self.repo.tags(limit.clamp(1, 100)).await
    }

    pub async fn get_by_slug(
        &self,
        slug: &str,
        actor: Option<&Actor>,
    ) -> Result<ShowcaseApp, DomainError> {
        let app = self
            .repo
            .get_by_slug(slug, actor.map(|a| a.email.as_str()))
            .await?;
        if !rules::can_view(actor, &app) {
            // Hidden apps look missing rather than forbidden.
            return Err(DomainError::NotFound);
        }
        Ok(
            if actor.is_some_and(|a| a.is_staff || rules::owns(a, &app)) {
                app
            } else {
                public(app)
            },
        )
    }

    /// More public apps that share the first technology, topped up with the latest.
    pub async fn related(&self, slug: &str, limit: i64) -> Result<Vec<ShowcaseApp>, DomainError> {
        let app = self.repo.get_by_slug(slug, None).await?;
        let mut q = ListQuery {
            technology: app.technologies.first().cloned(),
            exclude: Some(app.id),
            sort: Sort::Stars,
            limit,
            ..Default::default()
        };
        let (mut apps, _) = self.list_published(q.clone()).await?;
        if apps.len() < limit as usize {
            q.technology = None;
            q.sort = Sort::Newest;
            let (more, _) = self.list_published(q).await?;
            let seen: Vec<_> = apps.iter().map(|a| a.id).collect();
            apps.extend(more.into_iter().filter(|a| !seen.contains(&a.id)));
            apps.truncate(limit as usize);
        }
        Ok(apps)
    }

    pub async fn list_mine(&self, actor: &Actor) -> Result<Vec<ShowcaseApp>, DomainError> {
        let (apps, _) = self
            .repo
            .list(&ListQuery {
                creator_email: Some(actor.email.clone()),
                viewer_email: Some(actor.email.clone()),
                sort: Sort::Updated,
                limit: 100,
                ..Default::default()
            })
            .await?;
        Ok(apps)
    }

    pub async fn review_queue(&self, actor: &Actor) -> Result<Vec<ShowcaseApp>, DomainError> {
        require_staff(actor)?;
        let (apps, _) = self
            .repo
            .list(&ListQuery {
                status: Some(ShowcaseStatus::PendingReview),
                sort: Sort::Updated,
                limit: 100,
                ..Default::default()
            })
            .await?;
        Ok(apps)
    }

    pub async fn create(
        &self,
        actor: &Actor,
        input: ShowcaseAppInput,
    ) -> Result<ShowcaseApp, DomainError> {
        if !actor.is_staff && !actor.agent {
            let recent = self
                .repo
                .created_since(&actor.email, Utc::now() - Duration::hours(1))
                .await?;
            if recent >= rules::CREATES_PER_HOUR {
                return Err(DomainError::RateLimited(
                    "that is a lot of new apps in an hour — try again shortly".into(),
                ));
            }
        }
        let status = rules::status_on_save(actor.source(), input.submit);
        let base = slugify(&input.name);
        let base = if base == "insight" {
            "app".into()
        } else {
            base
        };
        let mut rec = record(&input, status)?;
        rec.app.id = Uuid::new_v4();
        rec.app.slug = next_free_slug(&base, &self.repo.slugs_like(&base).await?);
        rec.app.creator_email = Some(actor.email.clone());
        rec.app.creator_display_name = if actor.name.is_empty() {
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
        self.repo.get(rec.app.id).await
    }

    pub async fn update(
        &self,
        actor: &Actor,
        id: ShowcaseAppId,
        input: ShowcaseAppInput,
    ) -> Result<ShowcaseApp, DomainError> {
        let existing = self.repo.get(id).await?;
        rules::check_can_edit(actor, &existing)?;
        let status = rules::status_on_update(actor, &existing, &input);
        let mut rec = record(&input, status)?;
        rec.app.id = existing.id;
        rec.app.published_at = match status {
            ShowcaseStatus::Published => existing.published_at.or(Some(Utc::now())),
            _ => None,
        };
        self.repo.update(&rec).await?;
        self.repo.get(id).await
    }

    pub async fn delete(&self, actor: &Actor, id: ShowcaseAppId) -> Result<(), DomainError> {
        let existing = self.repo.get(id).await?;
        rules::check_can_edit(actor, &existing)?;
        self.repo.delete(id).await
    }

    pub async fn moderate(
        &self,
        actor: &Actor,
        id: ShowcaseAppId,
        action: Moderation,
    ) -> Result<ShowcaseApp, DomainError> {
        require_staff(actor)?;
        let app = self.repo.get(id).await?;
        let status = rules::moderate(app.status, &action)?;
        let featured = match action {
            Moderation::Feature(on) => on,
            _ if status == ShowcaseStatus::Published => app.featured,
            _ => false,
        };
        let note = match &action {
            Moderation::Reject { note } => note.trim().to_string(),
            _ => String::new(),
        };
        let published_at = match status {
            ShowcaseStatus::Published => app.published_at.or(Some(Utc::now())),
            _ => None,
        };
        self.repo
            .set_moderation(id, status, featured, &note, published_at)
            .await?;
        self.repo.get(id).await
    }

    /// Star or unstar an app the viewer can see. Returns the new count.
    pub async fn star(&self, actor: &Actor, slug: &str, on: bool) -> Result<i32, DomainError> {
        let app = self.get_by_slug(slug, Some(actor)).await?;
        if app.status != ShowcaseStatus::Published {
            return Err(DomainError::Conflict(
                "only published apps can be starred".into(),
            ));
        }
        self.repo.set_star(app.id, &actor.email, on).await
    }

    /// Dev/int only: fill an empty showcase with clearly-labelled samples.
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
            let app = self.create(&editor, input).await?;
            if featured {
                self.moderate(&editor, app.id, Moderation::Feature(true))
                    .await?;
            }
        }
        Ok(n)
    }
}

/// Validate and shape an input into what the repository stores. The caller
/// fills identity (id, slug, creator) and the source.
fn record(input: &ShowcaseAppInput, status: ShowcaseStatus) -> Result<AppRecord, DomainError> {
    let cleaned = rules::validate(input)?;
    let tags_text = rules::tags_text(&cleaned);
    let description_md = input.description_md.trim().to_string();
    let now = Utc::now();
    let t = |s: &str| s.trim().to_string();
    let mut evidence = input.evidence.clone();
    evidence.status_page_url = t(&evidence.status_page_url);
    Ok(AppRecord {
        app: ShowcaseApp {
            id: Uuid::nil(),
            slug: String::new(),
            name: t(&input.name),
            tagline: t(&input.tagline),
            description_html: markdown_to_html(&description_md),
            description_md,
            app_type: input.app_type.unwrap_or(ShowcaseAppType::Other),
            maturity: input.maturity.unwrap_or(ShowcaseMaturity::Prototype),
            build_method: input.build_method.unwrap_or(ShowcaseBuildMethod::HumanAi),
            pricing: input.pricing.unwrap_or(ShowcasePricing::Free),
            license: t(&input.license),
            version: t(&input.version),
            logo_url: t(&input.logo_url),
            screenshots: cleaned.screenshots,
            repo_url: t(&input.repo_url),
            demo_url: t(&input.demo_url),
            website_url: t(&input.website_url),
            docs_url: t(&input.docs_url),
            technologies: cleaned.technologies,
            ai_models: cleaned.ai_models,
            agents: cleaned.agents,
            human_oversight: t(&input.human_oversight),
            evidence,
            team_name: t(&input.team_name),
            creator_email: None,
            creator_display_name: String::new(),
            visibility: input.visibility.unwrap_or(ShowcaseVisibility::Public),
            status,
            verification: ShowcaseVerification::Unverified,
            featured: false,
            review_note: String::new(),
            star_count: 0,
            starred: false,
            published_at: (status == ShowcaseStatus::Published).then_some(now),
            created_at: now,
            updated_at: now,
        },
        source: jobshout_domain::InsightSource::Community,
        tags_text,
    })
}

fn require_staff(actor: &Actor) -> Result<(), DomainError> {
    if actor.is_staff {
        Ok(())
    } else {
        Err(DomainError::Forbidden("only editors can do that".into()))
    }
}

/// Strip what the public should not see.
fn public(mut app: ShowcaseApp) -> ShowcaseApp {
    app.creator_email = None;
    app.review_note = String::new();
    app
}
