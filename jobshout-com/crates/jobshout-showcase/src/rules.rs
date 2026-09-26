//! Pure business rules for the showcase: who may do what, what a valid app
//! looks like, and when a production claim is backed. No database here, so
//! every rule is unit-tested.

use jobshout_content::media::is_web_url;
use jobshout_content::render::word_count;
use jobshout_content::Actor;
use jobshout_domain::{
    DomainError, InsightSource, ProductionEvidence, ShowcaseAgent, ShowcaseApp, ShowcaseAppInput,
    ShowcaseKind, ShowcaseLinkInput, ShowcaseMaturity, ShowcaseStatus, ShowcaseVisibility,
    AGENT_CAPABILITIES,
};

pub const MAX_NAME: usize = 80;
pub const MAX_TAGLINE: usize = 140;
pub const MAX_DESCRIPTION: usize = 20_000;
pub const MIN_DESCRIPTION_WORDS: usize = 20;
pub const MAX_SCREENSHOTS: usize = 8;
pub const MAX_TAGS: usize = 15;
pub const MAX_TAG_LEN: usize = 32;
pub const MAX_MODELS: usize = 10;
pub const MAX_AGENTS: usize = 12;
pub const MAX_SHORT: usize = 80;
pub const MAX_OVERSIGHT: usize = 1_000;
pub const MAX_TOOLS: usize = 20;
pub const MAX_MCP: usize = 15;
pub const MIN_TEAM: usize = 2;
pub const MAX_JOBS: usize = 10;
/// Apps a community creator may add per rolling hour. Editors and agents are exempt.
pub const CREATES_PER_HOUR: i64 = 10;

/// The word for an entry of this kind, for messages.
pub fn noun(kind: ShowcaseKind) -> &'static str {
    match kind {
        ShowcaseKind::App => "app",
        ShowcaseKind::Agent => "agent",
        ShowcaseKind::Team => "team",
    }
}

pub fn owns(actor: &Actor, app: &ShowcaseApp) -> bool {
    app.creator_email
        .as_deref()
        .is_some_and(|e| e.eq_ignore_ascii_case(&actor.email))
}

/// Status an app lands in when saved. Only editors publish directly;
/// everyone else, agents included, goes through review.
pub fn status_on_save(source: InsightSource, submit: bool) -> ShowcaseStatus {
    match (source, submit) {
        (InsightSource::Staff, true) => ShowcaseStatus::Published,
        (InsightSource::Agent, _) => ShowcaseStatus::PendingReview,
        (_, true) => ShowcaseStatus::PendingReview,
        (_, false) => ShowcaseStatus::Draft,
    }
}

/// Status after an edit. A creator keeps a live app live while they fix
/// text, but anything that changes where visitors are sent (links, logo,
/// screenshots) goes back to review: approval covered the old destinations.
pub fn status_on_update(
    actor: &Actor,
    existing: &ShowcaseApp,
    input: &ShowcaseAppInput,
) -> ShowcaseStatus {
    if existing.status == ShowcaseStatus::Published && !actor.agent {
        if actor.is_staff || !links_changed(existing, input) {
            return ShowcaseStatus::Published;
        }
        return ShowcaseStatus::PendingReview;
    }
    status_on_save(actor.source(), input.submit)
}

fn links_changed(app: &ShowcaseApp, input: &ShowcaseAppInput) -> bool {
    let same = |a: &str, b: &str| a.trim() == b.trim();
    !same(&app.repo_url, &input.repo_url)
        || !same(&app.demo_url, &input.demo_url)
        || !same(&app.website_url, &input.website_url)
        || !same(&app.docs_url, &input.docs_url)
        || !same(&app.logo_url, &input.logo_url)
        || !same(
            &app.evidence.status_page_url,
            &input.evidence.status_page_url,
        )
        || clean_urls(&input.screenshots) != app.screenshots
}

/// Creators edit their own apps unless an editor archived them; editors edit anything.
pub fn check_can_edit(actor: &Actor, app: &ShowcaseApp) -> Result<(), DomainError> {
    if actor.is_staff {
        return Ok(());
    }
    if !owns(actor, app) {
        return Err(DomainError::Forbidden(format!(
            "you can only edit your own {}s",
            noun(app.kind)
        )));
    }
    if app.status == ShowcaseStatus::Archived {
        return Err(DomainError::Forbidden(format!(
            "this {} was archived by the editors",
            noun(app.kind)
        )));
    }
    Ok(())
}

/// Published non-private apps are visible to anyone with the link; the rest
/// only to their creator and editors.
pub fn can_view(actor: Option<&Actor>, app: &ShowcaseApp) -> bool {
    (app.status == ShowcaseStatus::Published && app.visibility != ShowcaseVisibility::Private)
        || actor.is_some_and(|a| a.is_staff || owns(a, app))
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Moderation {
    Approve,
    Reject { note: String },
    Feature(bool),
    Archive,
}

pub fn moderate(from: ShowcaseStatus, action: &Moderation) -> Result<ShowcaseStatus, DomainError> {
    use ShowcaseStatus::*;
    let bad = |what: &str| {
        Err(DomainError::Conflict(format!(
            "cannot {what} an app that is {}",
            from.as_str().replace('_', " ")
        )))
    };
    match action {
        Moderation::Approve => match from {
            PendingReview | Rejected | Archived => Ok(Published),
            _ => bad("approve"),
        },
        Moderation::Reject { note } => {
            if note.trim().is_empty() {
                return Err(DomainError::Validation(
                    "tell the creator what to change".into(),
                ));
            }
            match from {
                PendingReview => Ok(Rejected),
                _ => bad("reject"),
            }
        }
        Moderation::Feature(_) => match from {
            Published => Ok(Published),
            _ => bad("feature"),
        },
        Moderation::Archive => match from {
            Archived => bad("archive"),
            _ => Ok(Archived),
        },
    }
}

/// What a production claim is missing, as short human labels. Empty means
/// the claim is backed (by the creator's own word; see `verification`).
pub fn missing_evidence(
    maturity: ShowcaseMaturity,
    evidence: &ProductionEvidence,
    has_live_link: bool,
) -> Vec<&'static str> {
    if !maturity.needs_evidence() {
        return Vec::new();
    }
    let mut need = vec![
        (evidence.automated_tests, "automated tests"),
        (evidence.ci_cd, "CI/CD"),
        (evidence.security_scanning, "security scanning"),
        (evidence.monitoring, "monitoring"),
        (evidence.documentation, "documentation"),
        (has_live_link, "a live demo or website"),
    ];
    if maturity == ShowcaseMaturity::EnterpriseReady {
        need.extend([
            (evidence.dependency_scanning, "dependency scanning"),
            (evidence.backups, "backups and recovery"),
            (evidence.release_history, "a release history"),
        ]);
    }
    need.into_iter()
        .filter(|(ok, _)| !ok)
        .map(|(_, label)| label)
        .collect()
}

/// Input after trimming and de-duplication, ready to store.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Cleaned {
    pub screenshots: Vec<String>,
    pub technologies: Vec<String>,
    pub ai_models: Vec<String>,
    pub agents: Vec<ShowcaseAgent>,
    pub tools: Vec<String>,
    pub mcp_servers: Vec<String>,
    pub capabilities: Vec<String>,
    /// Directory links, de-duplicated by slug, in the order given.
    pub agent_links: Vec<ShowcaseLinkInput>,
    pub team_slug: Option<String>,
    /// Linked jobs, each once, in the order given.
    pub job_ids: Vec<uuid::Uuid>,
}

fn clean_urls(urls: &[String]) -> Vec<String> {
    urls.iter()
        .map(|u| u.trim().to_string())
        .filter(|u| !u.is_empty())
        .collect()
}

/// Trim, collapse inner whitespace, drop empties and case-insensitive
/// duplicates. The first spelling wins ("PostgreSQL" over "postgresql").
pub fn clean_tags(tags: &[String]) -> Vec<String> {
    let mut out: Vec<String> = Vec::new();
    for t in tags {
        let t = t.split_whitespace().collect::<Vec<_>>().join(" ");
        if !t.is_empty() && !out.iter().any(|o| o.eq_ignore_ascii_case(&t)) {
            out.push(t);
        }
    }
    out
}

/// Trimmed, lower-cased slug links with each slug kept once.
fn clean_links(links: &[ShowcaseLinkInput]) -> Vec<ShowcaseLinkInput> {
    let mut out: Vec<ShowcaseLinkInput> = Vec::new();
    for l in links {
        let slug = l.slug.trim().to_ascii_lowercase();
        if !slug.is_empty() && !out.iter().any(|o| o.slug == slug) {
            out.push(ShowcaseLinkInput {
                slug,
                role: l.role.trim().to_string(),
            });
        }
    }
    out
}

/// Check an input for an entry of `kind`. Drafts only need a name;
/// submitting needs a complete entry.
pub fn validate(kind: ShowcaseKind, input: &ShowcaseAppInput) -> Result<Cleaned, DomainError> {
    let v = |m: &str| Err(DomainError::Validation(m.to_string()));
    let name = input.name.trim();
    if name.is_empty() {
        return v("name is required");
    }
    if name.chars().count() > MAX_NAME {
        return v("name must be 80 characters or fewer");
    }
    if input.tagline.trim().chars().count() > MAX_TAGLINE {
        return v("tagline must be 140 characters or fewer");
    }
    if input.description_md.len() > MAX_DESCRIPTION {
        return v("description must be 20,000 characters or fewer");
    }
    for (label, value) in [
        ("licence", &input.license),
        ("version", &input.version),
        ("team", &input.team_name),
    ] {
        if value.trim().chars().count() > MAX_SHORT {
            return Err(DomainError::Validation(format!(
                "{label} must be 80 characters or fewer"
            )));
        }
    }
    if input.human_oversight.trim().chars().count() > MAX_OVERSIGHT {
        return v("human oversight must be 1,000 characters or fewer");
    }
    for (label, url) in [
        ("logo", &input.logo_url),
        ("repository link", &input.repo_url),
        ("demo link", &input.demo_url),
        ("website link", &input.website_url),
        ("docs link", &input.docs_url),
        ("status page link", &input.evidence.status_page_url),
    ] {
        if !url.trim().is_empty() && !is_web_url(url) {
            return Err(DomainError::Validation(format!(
                "{label} must be an http(s) URL"
            )));
        }
    }

    let screenshots = clean_urls(&input.screenshots);
    if screenshots.len() > MAX_SCREENSHOTS {
        return v("add at most eight screenshots");
    }
    if screenshots.iter().any(|u| !is_web_url(u)) {
        return v("screenshot must be an http(s) URL");
    }

    let technologies = clean_tags(&input.technologies);
    if technologies.len() > MAX_TAGS {
        return v("list at most 15 technologies");
    }
    let ai_models = clean_tags(&input.ai_models);
    if ai_models.len() > MAX_MODELS {
        return v("list at most 10 AI models");
    }
    if technologies
        .iter()
        .chain(&ai_models)
        .any(|t| t.chars().count() > MAX_TAG_LEN)
    {
        return v("technology and model names must be 32 characters or fewer");
    }

    let agents: Vec<ShowcaseAgent> = input
        .agents
        .iter()
        .map(|a| ShowcaseAgent {
            name: a.name.trim().to_string(),
            role: a.role.trim().to_string(),
        })
        .filter(|a| !a.name.is_empty())
        .collect();
    if agents.len() > MAX_AGENTS {
        return v("list at most 12 agents");
    }
    if agents
        .iter()
        .any(|a| a.name.chars().count() > 60 || a.role.chars().count() > MAX_SHORT)
    {
        return v("agent names must be 60 characters or fewer, and roles 80");
    }

    let tools = clean_tags(&input.tools);
    let mcp_servers = clean_tags(&input.mcp_servers);
    let capabilities: Vec<String> = clean_tags(&input.capabilities)
        .into_iter()
        .map(|c| c.to_ascii_lowercase())
        .collect();
    let agent_links = clean_links(&input.agent_links);
    let team_slug = Some(input.team_slug.trim().to_ascii_lowercase()).filter(|s| !s.is_empty());
    if kind != ShowcaseKind::Agent
        && (!tools.is_empty()
            || !mcp_servers.is_empty()
            || !capabilities.is_empty()
            || !input.model_provider.trim().is_empty())
    {
        return v("tools, MCP servers, capabilities and a model provider are for agents");
    }
    if kind == ShowcaseKind::Agent && !agent_links.is_empty() {
        return v("an agent cannot link other agents — put them in a team");
    }
    if kind != ShowcaseKind::App && team_slug.is_some() {
        return v("only apps can say which team built them");
    }
    if tools.len() > MAX_TOOLS {
        return v("list at most 20 tools");
    }
    if mcp_servers.len() > MAX_MCP {
        return v("list at most 15 MCP servers");
    }
    if tools
        .iter()
        .chain(&mcp_servers)
        .any(|t| t.chars().count() > MAX_TAG_LEN)
    {
        return v("tool and MCP server names must be 32 characters or fewer");
    }
    if let Some(c) = capabilities
        .iter()
        .find(|c| !AGENT_CAPABILITIES.contains(&c.as_str()))
    {
        return Err(DomainError::Validation(format!("unknown capability: {c}")));
    }
    if input.model_provider.trim().chars().count() > MAX_SHORT {
        return v("model provider must be 80 characters or fewer");
    }
    if agent_links.len() > MAX_AGENTS {
        return v("link at most 12 agents");
    }
    if agent_links
        .iter()
        .any(|l| l.role.chars().count() > MAX_SHORT)
    {
        return v("agent roles must be 80 characters or fewer");
    }
    let mut job_ids: Vec<uuid::Uuid> = Vec::new();
    for id in &input.job_ids {
        if !job_ids.contains(id) {
            job_ids.push(*id);
        }
    }
    if job_ids.len() > MAX_JOBS {
        return v("link at most 10 open roles");
    }

    let cleaned = Cleaned {
        screenshots,
        technologies,
        ai_models,
        agents,
        tools,
        mcp_servers,
        capabilities,
        agent_links,
        team_slug,
        job_ids,
    };
    if !input.submit {
        return Ok(cleaned);
    }

    if input.tagline.trim().is_empty() {
        return v("add a one-line tagline");
    }
    let words = word_count(&input.description_md);
    if words < MIN_DESCRIPTION_WORDS {
        return Err(DomainError::Validation(format!(
            "describe the {} in at least {MIN_DESCRIPTION_WORDS} words — this has {words}",
            noun(kind)
        )));
    }
    match kind {
        ShowcaseKind::App => {}
        ShowcaseKind::Agent => {
            if cleaned.capabilities.is_empty() {
                return v("pick at least one capability");
            }
            if cleaned.ai_models.is_empty() {
                return v("name the model the agent runs on");
            }
            return Ok(cleaned);
        }
        ShowcaseKind::Team => {
            if cleaned.agent_links.len() < MIN_TEAM {
                return v("a team needs at least two agents from the directory");
            }
            return Ok(cleaned);
        }
    }
    if input.app_type.is_none() {
        return v("pick what kind of app this is");
    }
    let Some(maturity) = input.maturity else {
        return v("pick how mature the app is");
    };
    let Some(build) = input.build_method else {
        return v("say how the app was built");
    };
    let live = !input.demo_url.trim().is_empty() || !input.website_url.trim().is_empty();
    if !live && input.repo_url.trim().is_empty() {
        return v("add a link to a repository, a demo or a website");
    }
    if build.is_agent_built()
        && cleaned.agents.is_empty()
        && cleaned.agent_links.is_empty()
        && cleaned.team_slug.is_none()
    {
        return v("name at least one agent that built it");
    }
    let missing = missing_evidence(maturity, &input.evidence, live);
    if !missing.is_empty() {
        return Err(DomainError::Validation(format!(
            "{} needs evidence: {}",
            maturity.as_str().replace('_', " "),
            missing.join(", ")
        )));
    }
    Ok(cleaned)
}

/// Words search should match beyond the name and description.
pub fn tags_text(cleaned: &Cleaned) -> String {
    cleaned
        .technologies
        .iter()
        .chain(&cleaned.ai_models)
        .chain(&cleaned.tools)
        .chain(&cleaned.mcp_servers)
        .chain(cleaned.agents.iter().map(|a| &a.name))
        .map(String::as_str)
        .collect::<Vec<_>>()
        .join(" ")
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;
    use jobshout_domain::{
        ShowcaseAppType, ShowcaseBuildMethod, ShowcasePricing, ShowcaseVerification,
    };
    use uuid::Uuid;

    fn app(status: ShowcaseStatus, creator: &str) -> ShowcaseApp {
        ShowcaseApp {
            id: Uuid::new_v4(),
            kind: ShowcaseKind::App,
            slug: "x".into(),
            name: "X".into(),
            tagline: String::new(),
            description_md: String::new(),
            description_html: String::new(),
            app_type: ShowcaseAppType::WebApplication,
            maturity: ShowcaseMaturity::Beta,
            build_method: ShowcaseBuildMethod::HumanAi,
            pricing: ShowcasePricing::Free,
            license: String::new(),
            version: String::new(),
            logo_url: String::new(),
            screenshots: vec![],
            repo_url: "https://github.com/acme/x".into(),
            demo_url: String::new(),
            website_url: String::new(),
            docs_url: String::new(),
            technologies: vec![],
            ai_models: vec![],
            agents: vec![],
            human_oversight: String::new(),
            evidence: ProductionEvidence::default(),
            team_name: String::new(),
            model_provider: String::new(),
            tools: vec![],
            mcp_servers: vec![],
            capabilities: vec![],
            linked_agents: vec![],
            linked_team: None,
            used_in: 0,
            jobs: vec![],
            creator_email: Some(creator.into()),
            creator_display_name: "C".into(),
            visibility: ShowcaseVisibility::Public,
            status,
            verification: ShowcaseVerification::Unverified,
            featured: false,
            review_note: String::new(),
            star_count: 0,
            starred: false,
            published_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        }
    }

    fn actor(email: &str, staff: bool) -> Actor {
        Actor {
            email: email.into(),
            name: "A".into(),
            is_staff: staff,
            agent: false,
        }
    }

    fn input() -> ShowcaseAppInput {
        ShowcaseAppInput {
            name: "NebulaDB".into(),
            tagline: "AI-native database for agents".into(),
            description_md: "word ".repeat(25),
            app_type: Some(ShowcaseAppType::DataPlatform),
            maturity: Some(ShowcaseMaturity::Beta),
            build_method: Some(ShowcaseBuildMethod::HumanAi),
            repo_url: "https://github.com/acme/x".into(),
            submit: true,
            ..Default::default()
        }
    }

    fn msg(r: Result<Cleaned, DomainError>) -> String {
        match r {
            Err(DomainError::Validation(m)) => m,
            other => panic!("expected a validation error, got {other:?}"),
        }
    }

    #[test]
    fn only_editors_publish_directly() {
        use InsightSource::*;
        assert_eq!(
            status_on_save(Community, true),
            ShowcaseStatus::PendingReview
        );
        assert_eq!(status_on_save(Community, false), ShowcaseStatus::Draft);
        assert_eq!(status_on_save(Agent, false), ShowcaseStatus::PendingReview);
        assert_eq!(status_on_save(Staff, true), ShowcaseStatus::Published);
        assert_eq!(status_on_save(Staff, false), ShowcaseStatus::Draft);
    }

    #[test]
    fn text_edits_stay_live_but_new_links_need_review() {
        let me = actor("me@example.com", false);
        let live = app(ShowcaseStatus::Published, "me@example.com");
        let mut edit = input();
        edit.repo_url = live.repo_url.clone();
        edit.description_md = "a better description ".repeat(5);
        assert_eq!(
            status_on_update(&me, &live, &edit),
            ShowcaseStatus::Published
        );

        edit.demo_url = "https://elsewhere.example".into();
        assert_eq!(
            status_on_update(&me, &live, &edit),
            ShowcaseStatus::PendingReview
        );

        let mut shots = input();
        shots.repo_url = live.repo_url.clone();
        shots.screenshots = vec!["https://img.example/1.png".into()];
        assert_eq!(
            status_on_update(&me, &live, &shots),
            ShowcaseStatus::PendingReview
        );

        // Editors keep it live whatever changed.
        assert_eq!(
            status_on_update(&actor("ed@jobshout.com", true), &live, &edit),
            ShowcaseStatus::Published
        );
        // Unpublished apps follow the normal save flow.
        let draft = app(ShowcaseStatus::Draft, "me@example.com");
        assert_eq!(
            status_on_update(&me, &draft, &edit),
            ShowcaseStatus::PendingReview
        );
    }

    #[test]
    fn agents_never_keep_an_app_live_on_edit() {
        let bot = Actor {
            email: "builder@agents.jobshout.com".into(),
            name: "Builder".into(),
            is_staff: false,
            agent: true,
        };
        let live = app(ShowcaseStatus::Published, "builder@agents.jobshout.com");
        let mut edit = input();
        edit.repo_url = live.repo_url.clone();
        assert_eq!(
            status_on_update(&bot, &live, &edit),
            ShowcaseStatus::PendingReview
        );
    }

    #[test]
    fn creators_edit_their_own_apps() {
        let me = actor("me@example.com", false);
        assert!(check_can_edit(&me, &app(ShowcaseStatus::Published, "ME@example.com")).is_ok());
        assert!(matches!(
            check_can_edit(&me, &app(ShowcaseStatus::Draft, "you@example.com")),
            Err(DomainError::Forbidden(_))
        ));
        assert!(matches!(
            check_can_edit(&me, &app(ShowcaseStatus::Archived, "me@example.com")),
            Err(DomainError::Forbidden(_))
        ));
        assert!(check_can_edit(
            &actor("ed@jobshout.com", true),
            &app(ShowcaseStatus::Archived, "me@example.com")
        )
        .is_ok());
    }

    #[test]
    fn visibility() {
        let me = actor("me@example.com", false);
        let other = actor("you@example.com", false);
        let mut a = app(ShowcaseStatus::Published, "me@example.com");
        assert!(can_view(None, &a));
        a.visibility = ShowcaseVisibility::Unlisted;
        assert!(can_view(None, &a), "unlisted is reachable by link");
        a.visibility = ShowcaseVisibility::Private;
        assert!(!can_view(None, &a));
        assert!(!can_view(Some(&other), &a));
        assert!(can_view(Some(&me), &a));
        let pending = app(ShowcaseStatus::PendingReview, "me@example.com");
        assert!(!can_view(Some(&other), &pending));
        assert!(can_view(Some(&actor("ed@jobshout.com", true)), &pending));
    }

    #[test]
    fn moderation_transitions() {
        use ShowcaseStatus::*;
        assert_eq!(
            moderate(PendingReview, &Moderation::Approve).unwrap(),
            Published
        );
        assert!(moderate(Draft, &Moderation::Approve).is_err());
        assert!(matches!(
            moderate(PendingReview, &Moderation::Reject { note: " ".into() }),
            Err(DomainError::Validation(_))
        ));
        assert_eq!(
            moderate(
                PendingReview,
                &Moderation::Reject {
                    note: "Add a demo".into()
                }
            )
            .unwrap(),
            Rejected
        );
        assert!(moderate(Draft, &Moderation::Feature(true)).is_err());
        assert_eq!(moderate(Published, &Moderation::Archive).unwrap(), Archived);
        assert!(moderate(Archived, &Moderation::Archive).is_err());
    }

    #[test]
    fn drafts_need_only_a_name() {
        let draft = ShowcaseAppInput {
            name: "Idea".into(),
            ..Default::default()
        };
        assert!(validate(ShowcaseKind::App, &draft).is_ok());
        assert!(msg(validate(ShowcaseKind::App, &ShowcaseAppInput::default())).contains("name"));
    }

    #[test]
    fn submitting_needs_a_complete_app() {
        assert!(validate(ShowcaseKind::App, &input()).is_ok());

        let mut i = input();
        i.tagline.clear();
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("tagline"));

        let mut i = input();
        i.description_md = "too short".into();
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("20 words"));

        let mut i = input();
        i.repo_url.clear();
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("repository, a demo or a website"));

        let mut i = input();
        i.maturity = None;
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("mature"));
    }

    #[test]
    fn production_ready_must_be_backed() {
        let mut i = input();
        i.maturity = Some(ShowcaseMaturity::ProductionReady);
        let m = msg(validate(ShowcaseKind::App, &i));
        assert!(m.starts_with("production ready needs evidence"), "{m}");
        assert!(m.contains("automated tests") && m.contains("a live demo or website"));

        i.demo_url = "https://demo.example".into();
        i.evidence = ProductionEvidence {
            automated_tests: true,
            ci_cd: true,
            security_scanning: true,
            monitoring: true,
            documentation: true,
            ..Default::default()
        };
        assert!(validate(ShowcaseKind::App, &i).is_ok());

        i.maturity = Some(ShowcaseMaturity::EnterpriseReady);
        let m = msg(validate(ShowcaseKind::App, &i));
        assert!(
            m.contains("dependency scanning") && m.contains("backups"),
            "{m}"
        );
    }

    #[test]
    fn agent_built_apps_name_their_agents() {
        let mut i = input();
        i.build_method = Some(ShowcaseBuildMethod::AgentBuilt);
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("agent"));
        i.agents = vec![ShowcaseAgent {
            name: "Rust Agent".into(),
            role: "Backend".into(),
        }];
        assert!(validate(ShowcaseKind::App, &i).is_ok());
    }

    #[test]
    fn links_must_be_web_urls() {
        let mut i = input();
        i.demo_url = "javascript:alert(1)".into();
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("demo link"));
        let mut i = input();
        i.screenshots = vec!["ftp://x/y.png".into()];
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("screenshot"));
        let mut i = input();
        i.screenshots = vec!["https://img.example/a.png".into(); 9];
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("eight"));
    }

    #[test]
    fn tags_are_trimmed_and_deduplicated() {
        let tags = ["  Rust ", "rust", "Postgre  SQL", "", "MCP"]
            .map(String::from)
            .to_vec();
        assert_eq!(clean_tags(&tags), vec!["Rust", "Postgre SQL", "MCP"]);

        let mut i = input();
        i.technologies = tags;
        i.ai_models = vec!["Claude".into()];
        i.agents = vec![
            ShowcaseAgent {
                name: " Rust Agent ".into(),
                role: "".into(),
            },
            ShowcaseAgent {
                name: "  ".into(),
                role: "dropped".into(),
            },
        ];
        let c = validate(ShowcaseKind::App, &i).unwrap();
        assert_eq!(c.agents.len(), 1);
        assert_eq!(tags_text(&c), "Rust Postgre SQL MCP Claude Rust Agent");
    }

    fn agent_input() -> ShowcaseAppInput {
        ShowcaseAppInput {
            kind: Some(ShowcaseKind::Agent),
            name: "Rust Backend Agent".into(),
            tagline: "Builds Axum services with tests".into(),
            description_md: "word ".repeat(25),
            ai_models: vec!["Claude".into()],
            capabilities: vec!["code_generation".into(), "Testing".into()],
            tools: vec!["GitHub".into(), "Docker".into()],
            submit: true,
            ..Default::default()
        }
    }

    fn link(slug: &str) -> ShowcaseLinkInput {
        ShowcaseLinkInput {
            slug: slug.into(),
            role: String::new(),
        }
    }

    #[test]
    fn agents_need_capabilities_and_a_model() {
        let c = validate(ShowcaseKind::Agent, &agent_input()).unwrap();
        assert_eq!(c.capabilities, vec!["code_generation", "testing"]);

        let mut i = agent_input();
        i.capabilities = vec![];
        assert!(msg(validate(ShowcaseKind::Agent, &i)).contains("capability"));
        let mut i = agent_input();
        i.capabilities = vec!["mind_reading".into()];
        assert!(msg(validate(ShowcaseKind::Agent, &i)).contains("unknown capability"));
        let mut i = agent_input();
        i.ai_models.clear();
        assert!(msg(validate(ShowcaseKind::Agent, &i)).contains("model"));
        // App-only rules do not apply to agents.
        let i = agent_input();
        assert!(i.repo_url.is_empty() && i.maturity.is_none());
        assert!(validate(ShowcaseKind::Agent, &i).is_ok());
    }

    #[test]
    fn agent_fields_are_for_agents_only() {
        let mut i = input();
        i.tools = vec!["Docker".into()];
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("for agents"));
        let mut i = agent_input();
        i.agent_links = vec![link("other-agent")];
        assert!(msg(validate(ShowcaseKind::Agent, &i)).contains("team"));
        let mut i = agent_input();
        i.team_slug = "some-team".into();
        assert!(msg(validate(ShowcaseKind::Agent, &i)).contains("only apps"));
    }

    #[test]
    fn teams_need_two_directory_agents() {
        let mut t = ShowcaseAppInput {
            kind: Some(ShowcaseKind::Team),
            name: "AI Startup Factory".into(),
            tagline: "Product to production".into(),
            description_md: "word ".repeat(25),
            agent_links: vec![link(" Product-Agent "), link("product-agent")],
            submit: true,
            ..Default::default()
        };
        assert!(msg(validate(ShowcaseKind::Team, &t)).contains("at least two"));
        t.agent_links.push(link("qa-agent"));
        let c = validate(ShowcaseKind::Team, &t).unwrap();
        let slugs: Vec<_> = c.agent_links.iter().map(|l| l.slug.as_str()).collect();
        assert_eq!(slugs, vec!["product-agent", "qa-agent"]);
    }

    #[test]
    fn a_linked_agent_or_team_counts_as_naming_one() {
        let mut i = input();
        i.build_method = Some(ShowcaseBuildMethod::AgentBuilt);
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("agent"));
        i.agent_links = vec![link("rust-agent")];
        assert!(validate(ShowcaseKind::App, &i).is_ok());
        i.agent_links.clear();
        i.team_slug = "startup-factory".into();
        assert_eq!(
            validate(ShowcaseKind::App, &i)
                .unwrap()
                .team_slug
                .as_deref(),
            Some("startup-factory")
        );
    }

    #[test]
    fn messages_name_the_kind() {
        let me = actor("me@example.com", false);
        let mut a = app(ShowcaseStatus::Draft, "you@example.com");
        a.kind = ShowcaseKind::Agent;
        match check_can_edit(&me, &a) {
            Err(DomainError::Forbidden(m)) => assert!(m.contains("agents"), "{m}"),
            other => panic!("{other:?}"),
        }
        let mut i = agent_input();
        i.description_md = "short".into();
        assert!(msg(validate(ShowcaseKind::Agent, &i)).contains("describe the agent"));
    }

    #[test]
    fn job_links_are_deduplicated_and_capped() {
        let a = uuid::Uuid::new_v4();
        let b = uuid::Uuid::new_v4();
        let mut i = input();
        i.job_ids = vec![a, b, a];
        assert_eq!(validate(ShowcaseKind::App, &i).unwrap().job_ids, vec![a, b]);
        i.job_ids = (0..11).map(|_| uuid::Uuid::new_v4()).collect();
        assert!(msg(validate(ShowcaseKind::App, &i)).contains("at most 10"));
        // Agents and teams can be hiring too.
        let mut t = agent_input();
        t.job_ids = vec![a];
        assert!(validate(ShowcaseKind::Agent, &t).is_ok());
    }
}
