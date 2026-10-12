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
use jobshout_domain::{DomainError, FeedItem, FollowState, FollowedEntry, Member, MemberInput};

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
        self.repo.set_headline(me.id, &headline).await?;
        self.member_by_email(&actor.email.to_ascii_lowercase())
            .await
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
