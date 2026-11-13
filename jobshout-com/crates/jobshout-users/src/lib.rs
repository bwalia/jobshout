//! Members and follows.
//!
//! A member is the public face of an account: a handle, a name and a one-line
//! headline, keyed privately by email (the only identity the web tier
//! forwards). Members are created on first need — signing in to follow,
//! publishing something — and backfilled at startup for everyone who already
//! has public work. People follow members and showcase entries; the feed is
//! read straight from the showcase and Insights tables, so nothing has to be
//! written when an entry is published or edited.

#![forbid(unsafe_code)]

mod repo;
pub mod rules;

use jobshout_content::Actor;
use jobshout_domain::{
    ConnectionStatus, Dashboard, DashboardAction, DashboardCredits, DashboardOpportunity,
    DashboardPipelineItem, DashboardStats, DomainError, FeedItem, FollowState, FollowedEntry,
    Member, MemberInput, RichMemberProfile, TalentSearchQuery,
};

use repo::MemberRepository;

/// How far back the feed reaches.
const FEED_DAYS: i64 = 90;

#[derive(Clone)]
pub struct MemberService {
    repo: MemberRepository,
}

impl MemberService {
    pub fn new(pool: sqlx::PgPool) -> Self {
        Self {
            repo: MemberRepository::new(pool),
        }
    }

    /// The actor's member, created on first call. A changed sign-in name
    /// renames them; the handle stays, so links keep working.
    pub async fn ensure(&self, actor: &Actor) -> Result<Member, DomainError> {
        let name = rules::display_name(&actor.name);
        let email = actor.email.trim().to_ascii_lowercase();
        if let Some(found) = self.repo.by_email(&email, Some(&email)).await? {
            if !actor.name.trim().is_empty() && found.0.display_name != name {
                self.repo.rename(found.0.id, &name).await?;
                return self.member_by_email(&email).await;
            }
            return Ok(found.0);
        }
        self.create(&email, &name, actor.agent).await?;
        self.member_by_email(&email).await
    }

    async fn member_by_email(&self, email: &str) -> Result<Member, DomainError> {
        Ok(self
            .repo
            .by_email(email, Some(email))
            .await?
            .ok_or(DomainError::NotFound)?
            .0)
    }

    /// Insert a member with the first free handle for `name`. Two sign-ins
    /// can race for a handle; the loser retries with the next one.
    async fn create(&self, email: &str, name: &str, is_agent: bool) -> Result<(), DomainError> {
        let base = rules::handle_base(name);
        for _ in 0..5 {
            let taken = self.repo.handles_like(&base).await?;
            let handle = rules::free_handle(&base, &taken);
            match self.repo.insert(&handle, email, name, is_agent).await {
                Err(DomainError::Conflict(_)) => continue,
                other => return other,
            }
        }
        Err(DomainError::Conflict(
            "could not pick a member handle — try again".into(),
        ))
    }

    /// Give everyone with public work a member, so their bylines link to a
    /// profile. Idempotent; run at startup.
    pub async fn backfill(&self) -> Result<usize, DomainError> {
        let missing = self.repo.authors_without_member().await?;
        let mut n = 0;
        for (email, name) in missing {
            let is_agent = rules::is_agent_email(&email);
            self.create(&email, &rules::display_name(&name), is_agent)
                .await?;
            n += 1;
        }
        Ok(n)
    }

    /// A member's public profile and the email behind it (for the caller to
    /// look up their work; never to be returned).
    pub async fn profile(
        &self,
        handle: &str,
        viewer: Option<&Actor>,
    ) -> Result<(Member, String), DomainError> {
        self.repo
            .by_handle(handle, viewer.map(|a| a.email.as_str()))
            .await?
            .ok_or(DomainError::NotFound)
    }

    /// The members directory: people and agents with public work, most
    /// followed first.
    pub async fn directory(
        &self,
        q: Option<String>,
        viewer: Option<&Actor>,
        limit: i64,
        offset: i64,
    ) -> Result<(Vec<Member>, i64), DomainError> {
        let q = q.map(|s| s.trim().to_string()).filter(|s| !s.is_empty());
        self.repo
            .directory(
                q.as_deref(),
                viewer.map(|a| a.email.as_str()),
                limit.clamp(1, 50),
                offset.max(0),
            )
            .await
    }

    pub async fn set_profile(
        &self,
        actor: &Actor,
        input: MemberInput,
    ) -> Result<Member, DomainError> {
        let headline = rules::clean_headline(&input.headline)?;
        let me = self.ensure(actor).await?;
        self.repo
            .update_rich_profile(me.id, &input, &headline)
            .await?;
        self.member_by_email(&actor.email.to_ascii_lowercase())
            .await
    }

    /// Full profile payload for `/members/{handle}` tabs.
    pub async fn rich_profile(
        &self,
        handle: &str,
        viewer: Option<&Actor>,
    ) -> Result<(RichMemberProfile, String), DomainError> {
        let (member, email) = self.profile(handle, viewer).await?;
        let id = member.id;
        Ok((
            RichMemberProfile {
                experience: self.repo.list_experience(id).await?,
                education: self.repo.list_education(id).await?,
                certifications: self.repo.list_certifications(id).await?,
                portfolio: self.repo.list_portfolio(id).await?,
                skills: self.repo.list_skills(id).await?,
                reviews: self.repo.list_reviews(id).await?,
                achievements: self.repo.list_achievements(id).await?,
                member,
            },
            email,
        ))
    }

    pub async fn talent_search(
        &self,
        query: TalentSearchQuery,
        viewer: Option<&Actor>,
    ) -> Result<(Vec<Member>, i64), DomainError> {
        let q = query.q.as_deref().map(str::trim).filter(|s| !s.is_empty());
        let availability = query
            .availability
            .as_deref()
            .map(str::trim)
            .filter(|s| !s.is_empty());
        self.repo
            .talent_search(&query, q, availability, viewer.map(|a| a.email.as_str()))
            .await
    }

    pub async fn connect(
        &self,
        actor: &Actor,
        handle: &str,
    ) -> Result<ConnectionStatus, DomainError> {
        let me = self.ensure(actor).await?;
        let (them, _) = self.profile(handle, Some(actor)).await?;
        self.repo.request_connection(me.id, them.id).await
    }

    pub async fn respond_connect(
        &self,
        actor: &Actor,
        handle: &str,
        accept: bool,
    ) -> Result<ConnectionStatus, DomainError> {
        let me = self.ensure(actor).await?;
        let (them, _) = self.profile(handle, Some(actor)).await?;
        self.repo.respond_connection(me.id, them.id, accept).await
    }

    pub async fn request_gdpr(&self, actor: &Actor, kind: &str) -> Result<uuid::Uuid, DomainError> {
        if kind != "export" && kind != "delete" {
            return Err(DomainError::Validation(
                "kind must be export or delete".into(),
            ));
        }
        let me = self.ensure(actor).await?;
        self.repo.create_gdpr_request(me.id, kind).await
    }

    pub async fn save_dashboard_widgets(
        &self,
        actor: &Actor,
        widgets: Vec<String>,
    ) -> Result<Vec<String>, DomainError> {
        let me = self.ensure(actor).await?;
        let cleaned: Vec<String> = widgets
            .into_iter()
            .map(|w| w.trim().to_string())
            .filter(|w| !w.is_empty())
            .take(20)
            .collect();
        self.repo.save_dashboard_layout(me.id, &cleaned).await?;
        Ok(cleaned)
    }

    /// Command-centre payload. AI brief / credits / approvals are stubs until
    /// later phases wire real data; the shape is stable.
    pub async fn dashboard(&self, actor: &Actor) -> Result<Dashboard, DomainError> {
        let me = self.ensure(actor).await?;
        let widgets = self.repo.dashboard_layout(me.id).await?;
        let mut today = Vec::new();
        if me.headline.is_empty() {
            today.push(DashboardAction {
                id: "headline".into(),
                title: "Add a headline".into(),
                reason: "Profiles with a headline are found more often in talent search.".into(),
                href: "/members/me".into(),
                kind: "profile".into(),
            });
        }
        if me.bio.is_empty() {
            today.push(DashboardAction {
                id: "bio".into(),
                title: "Write a short bio".into(),
                reason: "A bio helps hirers understand what you do in one glance.".into(),
                href: "/dashboard".into(),
                kind: "profile".into(),
            });
        }
        let skills = self.repo.list_skills(me.id).await?;
        if skills.is_empty() {
            today.push(DashboardAction {
                id: "skills".into(),
                title: "Add skills with evidence".into(),
                reason: "Skill evidence unlocks better project and job matches.".into(),
                href: format!("/members/{}", me.handle),
                kind: "profile".into(),
            });
        }
        if today.is_empty() {
            today.push(DashboardAction {
                id: "browse-projects".into(),
                title: "Browse open projects".into(),
                reason: "Stay visible — check new work that fits your skills.".into(),
                href: "/projects".into(),
                kind: "opportunity".into(),
            });
        }

        let mut fixes = Vec::new();
        let mut strength = 40;
        if !me.headline.is_empty() {
            strength += 10;
        } else {
            fixes.push("Add a headline".into());
        }
        if !me.bio.is_empty() {
            strength += 10;
        } else {
            fixes.push("Add a bio".into());
        }
        if !skills.is_empty() {
            strength += 15;
        } else {
            fixes.push("Add at least one skill".into());
        }
        if !me.avatar_url.is_empty() {
            strength += 10;
        } else {
            fixes.push("Upload an avatar".into());
        }
        if me.open_to_work || me.open_to_hire {
            strength += 5;
        }
        strength = strength.min(100);

        let opportunities = if me.open_to_work || !skills.is_empty() {
            vec![DashboardOpportunity {
                id: "projects-radar".into(),
                kind: "project".into(),
                title: "Projects matching your profile".into(),
                summary: "Browse live projects once PROJECTS_LIVE is on.".into(),
                href: "/projects".into(),
                score: 0.72,
                reasons: vec![
                    "Based on your skills and availability".into(),
                    "Budgets in a typical freelancer range".into(),
                ],
            }]
        } else {
            Vec::new()
        };

        Ok(Dashboard {
            today,
            opportunities,
            pipeline: vec![DashboardPipelineItem {
                id: "applications".into(),
                kind: "application".into(),
                title: "Job applications".into(),
                stage: "track".into(),
                next_step: "Review status".into(),
                href: "/applications".into(),
            }],
            stats: DashboardStats {
                profile_views_7d: 0,
                search_appearances_7d: 0,
                follower_growth_7d: 0,
                invitation_rate: 0.0,
                proposal_win_rate: 0.0,
                profile_strength: strength,
                profile_fixes: fixes,
            },
            credits: DashboardCredits {
                balance: 0,
                expiring_soon: 0,
                plan: "free".into(),
            },
            earnings_reported: 0.0,
            unread_messages: 0,
            unread_notifications: 0,
            approval_inbox: Vec::new(),
            weekly_goals: vec![DashboardAction {
                id: "goal-proposals".into(),
                title: "Send 5 proposals this week".into(),
                reason: "Steady outreach compounds.".into(),
                href: "/projects".into(),
                kind: "goal".into(),
            }],
            roles: vec!["member".into()],
            widgets,
        })
    }

    pub async fn follow(
        &self,
        actor: &Actor,
        handle: &str,
        on: bool,
    ) -> Result<FollowState, DomainError> {
        let (member, email) = self.profile(handle, Some(actor)).await?;
        if on && email.eq_ignore_ascii_case(actor.email.trim()) {
            return Err(DomainError::Validation("you can't follow yourself".into()));
        }
        let follower_count = self.repo.set_follow(member.id, &actor.email, on).await?;
        Ok(FollowState {
            following: on,
            follower_count,
        })
    }

    /// Who and what the actor follows.
    pub async fn following(
        &self,
        actor: &Actor,
    ) -> Result<(Vec<Member>, Vec<FollowedEntry>), DomainError> {
        let members = self.repo.followed_members(&actor.email).await?;
        let entries = self.repo.followed_entries(&actor.email).await?;
        Ok((members, entries))
    }

    /// The actor's feed, newest first: what followed members published, and
    /// what happened to followed entries. `before` pages back.
    pub async fn feed(
        &self,
        actor: &Actor,
        before: Option<chrono::DateTime<chrono::Utc>>,
        limit: i64,
    ) -> Result<Vec<FeedItem>, DomainError> {
        let since = chrono::Utc::now() - chrono::Duration::days(FEED_DAYS);
        let limit = limit.clamp(1, 100);
        // Fetch a little extra: dedupe can drop some.
        let items = self
            .repo
            .feed(&actor.email, since, before, limit + 10)
            .await?;
        let mut out = rules::dedupe_feed(items);
        out.truncate(limit as usize);
        Ok(out)
    }
}
