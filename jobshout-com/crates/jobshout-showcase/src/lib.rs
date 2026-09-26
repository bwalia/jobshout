//! AI Showcase: applications built by people and AI agents, and the
//! directory of those agents and agent teams (one table, `kind` apart).
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
    ShowcaseBuildMethod, ShowcaseKind, ShowcaseMaturity, ShowcasePricing, ShowcaseStatus,
    ShowcaseTag, ShowcaseVerification, ShowcaseVisibility,
};
use sqlx::PgPool;
use uuid::Uuid;

use repo::{AppRecord, LinkTarget, ShowcaseRepository};
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

    /// The public list: published, public entries of one kind (apps unless
    /// asked otherwise), creator emails removed.
    pub async fn list_published(
        &self,
        mut q: ListQuery,
    ) -> Result<(Vec<ShowcaseApp>, i64), DomainError> {
        q.kind = Some(q.kind.unwrap_or(ShowcaseKind::App));
        q.capability = q.capability.filter(|s| !s.trim().is_empty());
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

    pub async fn tags(
        &self,
        kind: ShowcaseKind,
        limit: i64,
    ) -> Result<Vec<ShowcaseTag>, DomainError> {
        self.repo.tags(kind, limit.clamp(1, 100)).await
    }

    /// Public apps and teams that link to an agent or team, for its profile.
    pub async fn used_in(
        &self,
        slug: &str,
        actor: Option<&Actor>,
    ) -> Result<(Vec<ShowcaseApp>, Vec<ShowcaseApp>), DomainError> {
        let entry = self.get_by_slug(slug, actor).await?;
        if entry.kind == ShowcaseKind::App {
            return Ok((Vec::new(), Vec::new()));
        }
        let q = |kind| ListQuery {
            kind: Some(kind),
            links_to: Some(entry.id),
            sort: Sort::Stars,
            limit: 24,
            ..Default::default()
        };
        let (apps, _) = self.list_published(q(ShowcaseKind::App)).await?;
        let teams = if entry.kind == ShowcaseKind::Agent {
            self.list_published(q(ShowcaseKind::Team)).await?.0
        } else {
            Vec::new()
        };
        Ok((apps, teams))
    }

    /// What a creator can link from the form: published public entries of
    /// `kind`, plus their own of any status.
    pub async fn link_candidates(
        &self,
        actor: &Actor,
        kind: ShowcaseKind,
        q: Option<String>,
    ) -> Result<Vec<ShowcaseApp>, DomainError> {
        let q = q.filter(|s| !s.trim().is_empty());
        let (mut out, _) = self
            .repo
            .list(&ListQuery {
                kind: Some(kind),
                creator_email: Some(actor.email.clone()),
                viewer_email: Some(actor.email.clone()),
                q: q.clone(),
                sort: Sort::Updated,
                limit: 10,
                ..Default::default()
            })
            .await?;
        let (public_ones, _) = self
            .list_published(ListQuery {
                kind: Some(kind),
                q,
                sort: Sort::Stars,
                limit: 20,
                ..Default::default()
            })
            .await?;
        for p in public_ones {
            if !out.iter().any(|o| o.id == p.id) {
                out.push(p);
            }
        }
        out.truncate(20);
        Ok(out.into_iter().map(public).collect())
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

    /// More public entries of the same kind that share the first
    /// technology, topped up with the latest.
    pub async fn related(&self, slug: &str, limit: i64) -> Result<Vec<ShowcaseApp>, DomainError> {
        let app = self.repo.get_by_slug(slug, None).await?;
        let mut q = ListQuery {
            kind: Some(app.kind),
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
                    "that is a lot of new listings in an hour — try again shortly".into(),
                ));
            }
        }
        let kind = input.kind.unwrap_or(ShowcaseKind::App);
        let status = rules::status_on_save(actor.source(), input.submit);
        let base = slugify(&input.name);
        let base = if base == "insight" {
            rules::noun(kind).into()
        } else {
            base
        };
        let mut rec = self.record(actor, kind, None, &input, status).await?;
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
        self.repo.get(rec.app.id, Some(&actor.email)).await
    }

    pub async fn update(
        &self,
        actor: &Actor,
        id: ShowcaseAppId,
        input: ShowcaseAppInput,
    ) -> Result<ShowcaseApp, DomainError> {
        let existing = self.repo.get(id, Some(&actor.email)).await?;
        rules::check_can_edit(actor, &existing)?;
        if input.kind.is_some_and(|k| k != existing.kind) {
            return Err(DomainError::Validation(format!(
                "this {} cannot be changed into another kind of listing",
                rules::noun(existing.kind)
            )));
        }
        let status = rules::status_on_update(actor, &existing, &input);
        let mut rec = self
            .record(actor, existing.kind, Some(existing.id), &input, status)
            .await?;
        rec.app.id = existing.id;
        rec.app.published_at = match status {
            ShowcaseStatus::Published => existing.published_at.or(Some(Utc::now())),
            _ => None,
        };
        self.repo.update(&rec).await?;
        self.repo.get(id, Some(&actor.email)).await
    }

    pub async fn delete(&self, actor: &Actor, id: ShowcaseAppId) -> Result<(), DomainError> {
        let existing = self.repo.get(id, None).await?;
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
        let app = self.repo.get(id, None).await?;
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
        self.repo.get(id, Some(&actor.email)).await
    }

    /// Star or unstar an app the viewer can see. Returns the new count.
    pub async fn star(&self, actor: &Actor, slug: &str, on: bool) -> Result<i32, DomainError> {
        let app = self.get_by_slug(slug, Some(actor)).await?;
        if app.status != ShowcaseStatus::Published {
            return Err(DomainError::Conflict(format!(
                "only published {}s can be starred",
                rules::noun(app.kind)
            )));
        }
        self.repo.set_star(app.id, &actor.email, on).await
    }

    /// Dev/int only: fill an empty showcase with clearly-labelled samples.
    /// Each kind seeds on its own, so an int that already has sample apps
    /// still gets sample agents, and the sample apps are linked to them.
    pub async fn seed_samples(&self) -> Result<usize, DomainError> {
        let editor = Actor {
            email: seed::EDITOR.into(),
            name: "JobShout Editors".into(),
            is_staff: true,
            agent: false,
        };
        let mut n = 0;
        if self.repo.count(ShowcaseKind::App).await? == 0 {
            for (input, featured) in seed::samples() {
                let app = self.create(&editor, input).await?;
                if featured {
                    self.moderate(&editor, app.id, Moderation::Feature(true))
                        .await?;
                }
                n += 1;
            }
        }
        if self.repo.count(ShowcaseKind::Agent).await? == 0 {
            for input in seed::agents().into_iter().chain(seed::teams()) {
                self.create(&editor, input).await?;
                n += 1;
            }
            // Link the sample apps (if they are still the editors' samples) to
            // the sample agents and team.
            for (app_slug, links) in seed::app_links() {
                let Ok(app) = self.repo.get_by_slug(app_slug, None).await else {
                    continue;
                };
                if !rules::owns(&editor, &app) {
                    continue;
                }
                let slugs: Vec<String> = links.iter().map(|(s, _)| s.to_string()).collect();
                let targets = self.repo.resolve_links(&slugs).await?;
                let resolved: Vec<_> = links
                    .iter()
                    .filter_map(|(slug, role)| {
                        targets
                            .iter()
                            .find(|t| t.slug == *slug)
                            .map(|t| (t.id, role.to_string()))
                    })
                    .collect();
                self.repo.replace_links(app.id, &resolved).await?;
            }
        }
        Ok(n)
    }

    /// Validate and shape an input into what the repository stores,
    /// resolving its directory links. The caller fills identity (id, slug,
    /// creator) and the source.
    async fn record(
        &self,
        actor: &Actor,
        kind: ShowcaseKind,
        self_id: Option<ShowcaseAppId>,
        input: &ShowcaseAppInput,
        status: ShowcaseStatus,
    ) -> Result<AppRecord, DomainError> {
        let cleaned = rules::validate(kind, input)?;
        let mut slugs: Vec<String> = cleaned.agent_links.iter().map(|l| l.slug.clone()).collect();
        slugs.extend(cleaned.team_slug.clone());
        let targets = self.repo.resolve_links(&slugs).await?;
        let mut links = Vec::new();
        for l in &cleaned.agent_links {
            let t = linkable(actor, &targets, &l.slug, ShowcaseKind::Agent, self_id)?;
            links.push((t.id, l.role.clone()));
        }
        if let Some(team) = &cleaned.team_slug {
            let t = linkable(actor, &targets, team, ShowcaseKind::Team, self_id)?;
            links.push((t.id, String::new()));
        }
        let mut rec = record(kind, input, cleaned, status);
        rec.links = links;
        Ok(rec)
    }
}

/// A link target the actor may use: it exists, is the right kind, is not the
/// entry itself, and is either public and published or the actor's own.
fn linkable<'a>(
    actor: &Actor,
    targets: &'a [LinkTarget],
    slug: &str,
    kind: ShowcaseKind,
    self_id: Option<ShowcaseAppId>,
) -> Result<&'a LinkTarget, DomainError> {
    let missing = || {
        DomainError::Validation(format!(
            "no {} called \"{slug}\" in the directory",
            rules::noun(kind)
        ))
    };
    let t = targets
        .iter()
        .find(|t| t.slug == slug && t.kind == kind)
        .ok_or_else(missing)?;
    if Some(t.id) == self_id {
        return Err(DomainError::Validation(format!(
            "a {} cannot link to itself",
            rules::noun(kind)
        )));
    }
    let visible =
        t.status == ShowcaseStatus::Published && t.visibility != ShowcaseVisibility::Private;
    if visible || actor.is_staff || t.creator_email.eq_ignore_ascii_case(&actor.email) {
        Ok(t)
    } else {
        Err(missing())
    }
}

/// Shape a validated input into what the repository stores.
fn record(
    kind: ShowcaseKind,
    input: &ShowcaseAppInput,
    cleaned: rules::Cleaned,
    status: ShowcaseStatus,
) -> AppRecord {
    let tags_text = rules::tags_text(&cleaned);
    let description_md = input.description_md.trim().to_string();
    let now = Utc::now();
    let t = |s: &str| s.trim().to_string();
    let mut evidence = input.evidence.clone();
    evidence.status_page_url = t(&evidence.status_page_url);
    AppRecord {
        app: ShowcaseApp {
            id: Uuid::nil(),
            kind,
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
            model_provider: t(&input.model_provider),
            tools: cleaned.tools,
            mcp_servers: cleaned.mcp_servers,
            capabilities: cleaned.capabilities,
            linked_agents: Vec::new(),
            linked_team: None,
            used_in: 0,
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
        links: Vec::new(),
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
fn public(mut app: ShowcaseApp) -> ShowcaseApp {
    app.creator_email = None;
    app.review_note = String::new();
    app
}
