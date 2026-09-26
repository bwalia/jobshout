//! Pure business rules: who may do what, what a valid item looks like.
//! Kept free of the database so every rule is unit-tested.

use std::collections::HashSet;

use chrono::{Datelike, NaiveDate};
use jobshout_domain::{
    DomainError, Insight, InsightInput, InsightKind, InsightSource, InsightStatus, NewsletterDigest,
};

use crate::media::{self, MediaFormat};
use crate::render::word_count;

pub const MAX_TITLE: usize = 160;
pub const MAX_SUMMARY: usize = 300;
pub const MAX_POST_WORDS: usize = 500;
pub const MIN_LONGFORM_WORDS: usize = 80;
pub const MAX_TOPICS: usize = 3;
/// Items a community author may create per rolling hour. Staff and agents are
/// exempt: they are trusted callers, and an agent run files a whole batch.
pub const CREATES_PER_HOUR: i64 = 10;

/// The person behind a request, as vouched for by the web tier.
#[derive(Debug, Clone)]
pub struct Actor {
    pub email: String,
    pub name: String,
    pub is_staff: bool,
    /// An automated author (e.g. the Article Writer). Never staff: whatever it
    /// writes goes through review.
    pub agent: bool,
}

impl Actor {
    pub fn owns(&self, item: &Insight) -> bool {
        item.author_email
            .as_deref()
            .is_some_and(|e| e.eq_ignore_ascii_case(&self.email))
    }

    pub fn source(&self) -> InsightSource {
        if self.agent {
            InsightSource::Agent
        } else if self.is_staff {
            InsightSource::Staff
        } else {
            InsightSource::Community
        }
    }
}

/// Status an item lands in when saved. Only staff publish directly; everyone
/// else — agents included — goes through the review queue.
pub fn status_on_save(source: InsightSource, submit: bool) -> InsightStatus {
    match (source, submit) {
        (InsightSource::Staff, true) => InsightStatus::Published,
        (InsightSource::Agent, _) => InsightStatus::PendingReview,
        (_, true) => InsightStatus::PendingReview,
        (_, false) => InsightStatus::Draft,
    }
}

/// Status a newly created item lands in. As `status_on_save`, except that an
/// agent jobshout.com trusts (INSIGHTS_TRUSTED_AGENTS) publishes a submitted
/// item directly: its platform only sends an item after a person approved it.
pub fn status_on_create(source: InsightSource, submit: bool, trusted_agent: bool) -> InsightStatus {
    match (source, submit, trusted_agent) {
        (InsightSource::Agent, true, true) => InsightStatus::Published,
        _ => status_on_save(source, submit),
    }
}

/// Authors edit their own work until it is published; staff edit anything.
pub fn check_can_edit(actor: &Actor, item: &Insight) -> Result<(), DomainError> {
    if actor.is_staff {
        return Ok(());
    }
    if !actor.owns(item) {
        return Err(DomainError::Forbidden(
            "you can only edit your own insights".into(),
        ));
    }
    match item.status {
        InsightStatus::Draft | InsightStatus::PendingReview | InsightStatus::Rejected => Ok(()),
        _ => Err(DomainError::Forbidden(
            "published insights can only be changed by the editors".into(),
        )),
    }
}

/// Non-published items are visible to their author and staff only.
pub fn can_view(actor: Option<&Actor>, item: &Insight) -> bool {
    item.status == InsightStatus::Published || actor.is_some_and(|a| a.is_staff || a.owns(item))
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Moderation {
    Approve,
    Reject { note: String },
    Feature(bool),
    Archive,
}

pub fn moderate(from: InsightStatus, action: &Moderation) -> Result<InsightStatus, DomainError> {
    use InsightStatus::*;
    let bad = |what: &str| {
        Err(DomainError::Conflict(format!(
            "cannot {what} an insight that is {}",
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
                    "tell the author why it was rejected".into(),
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
            Published | PendingReview | Rejected | Draft => Ok(Archived),
            Archived => bad("archive"),
        },
    }
}

/// Validated media fields for an item.
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct ResolvedMedia {
    pub provider: String,
    pub embed_url: String,
}

/// Check an input. Drafts only need a title; submitting needs a complete item.
pub fn validate(
    kind: InsightKind,
    input: &InsightInput,
    known_topics: &HashSet<String>,
) -> Result<ResolvedMedia, DomainError> {
    let v = |m: &str| Err(DomainError::Validation(m.to_string()));
    let title = input.title.trim();
    if title.is_empty() {
        return v("title is required");
    }
    if title.chars().count() > MAX_TITLE {
        return v("title must be 160 characters or fewer");
    }
    if input.summary.trim().chars().count() > MAX_SUMMARY {
        return v("summary must be 300 characters or fewer");
    }
    if !input.cover_image_url.trim().is_empty() && !media::is_web_url(&input.cover_image_url) {
        return v("cover image must be an http(s) URL");
    }
    if !input.link_url.trim().is_empty() && !media::is_web_url(&input.link_url) {
        return v("link must be an http(s) URL");
    }
    if let Some(d) = input.duration_seconds {
        if !(0..=86_400).contains(&d) {
            return v("duration must be between 0 and 24 hours");
        }
    }
    if input.topics.len() > MAX_TOPICS {
        return v("pick at most three topics");
    }
    if let Some(t) = input.topics.iter().find(|t| !known_topics.contains(*t)) {
        return Err(DomainError::Validation(format!("unknown topic: {t}")));
    }

    let media = if input.media_url.trim().is_empty() {
        ResolvedMedia::default()
    } else {
        let m = media::resolve(&input.media_url).map_err(DomainError::Validation)?;
        let fits = match kind {
            InsightKind::Video => m.format != MediaFormat::Audio,
            InsightKind::Podcast => m.format != MediaFormat::Video || m.provider == "file",
            _ => true,
        };
        if !fits {
            return Err(DomainError::Validation(format!(
                "that link is {} but this is a {}",
                if m.format == MediaFormat::Audio {
                    "audio"
                } else {
                    "video"
                },
                kind.as_str()
            )));
        }
        ResolvedMedia {
            provider: m.provider.to_string(),
            embed_url: m.embed_url,
        }
    };

    let words = word_count(&input.body_md);
    if kind == InsightKind::Post && words > MAX_POST_WORDS {
        return v("posts are short — 500 words at most. Make it an article instead.");
    }

    if !input.submit {
        return Ok(media);
    }

    if input.topics.is_empty() {
        return v("pick at least one topic");
    }
    if !input.cover_image_url.trim().is_empty() && input.cover_image_alt.trim().is_empty() {
        return v("describe the cover image for people using screen readers");
    }
    match kind {
        InsightKind::Post => {
            if words == 0 && input.link_url.trim().is_empty() {
                return v("a post needs some text or a link");
            }
        }
        InsightKind::Article | InsightKind::Blog => {
            if words < MIN_LONGFORM_WORDS {
                return Err(DomainError::Validation(format!(
                    "a {} needs at least {MIN_LONGFORM_WORDS} words — this has {words}",
                    kind.as_str()
                )));
            }
        }
        InsightKind::Podcast | InsightKind::Video => {
            if media.embed_url.is_empty() {
                return Err(DomainError::Validation(format!(
                    "a {} needs a media link",
                    kind.as_str()
                )));
            }
        }
    }
    Ok(media)
}

pub fn slugify(title: &str) -> String {
    let mut out = String::with_capacity(title.len());
    let mut dash = false;
    for c in title.chars().flat_map(char::to_lowercase) {
        if c.is_ascii_alphanumeric() {
            out.push(c);
            dash = false;
        } else if !dash && !out.is_empty() {
            out.push('-');
            dash = true;
        }
        if out.len() >= 80 {
            break;
        }
    }
    let out = out.trim_matches('-').to_string();
    if out.is_empty() {
        "insight".into()
    } else {
        out
    }
}

/// First free slug: `base`, then `base-2`, `base-3`, …
pub fn next_free_slug(base: &str, taken: &HashSet<String>) -> String {
    if !taken.contains(base) {
        return base.to_string();
    }
    (2..)
        .map(|n| format!("{base}-{n}"))
        .find(|s| !taken.contains(s))
        .expect("unbounded range")
}

pub fn is_email(s: &str) -> bool {
    let s = s.trim();
    let Some((local, domain)) = s.split_once('@') else {
        return false;
    };
    !local.is_empty()
        && domain.contains('.')
        && !domain.starts_with('.')
        && !domain.ends_with('.')
        && !s.chars().any(char::is_whitespace)
        && s.len() <= 254
}

/// Group published items into ISO weeks, newest week first.
pub fn group_by_week(items: Vec<Insight>) -> Vec<NewsletterDigest> {
    let mut weeks: Vec<NewsletterDigest> = Vec::new();
    for item in items {
        let Some(at) = item.published_at else {
            continue;
        };
        let iso = at.date_naive().iso_week();
        let label = format!("{}-W{:02}", iso.year(), iso.week());
        match weeks.iter_mut().find(|w| w.week == label) {
            Some(w) => w.items.push(item),
            None => weeks.push(NewsletterDigest {
                week: label,
                starts_on: NaiveDate::from_isoywd_opt(iso.year(), iso.week(), chrono::Weekday::Mon)
                    .expect("valid iso week"),
                items: vec![item],
            }),
        }
    }
    weeks.sort_by_key(|w| std::cmp::Reverse(w.starts_on));
    for w in &mut weeks {
        w.items.sort_by_key(|i| std::cmp::Reverse(i.published_at));
    }
    weeks
}

#[cfg(test)]
pub(crate) mod tests {
    use super::*;
    use chrono::{TimeZone, Utc};
    use uuid::Uuid;

    pub fn item(status: InsightStatus, author: &str) -> Insight {
        Insight {
            id: Uuid::new_v4(),
            kind: InsightKind::Article,
            slug: "x".into(),
            title: "X".into(),
            summary: String::new(),
            body_md: String::new(),
            body_html: String::new(),
            cover_image_url: String::new(),
            cover_image_alt: String::new(),
            link_url: String::new(),
            media_url: String::new(),
            embed_provider: String::new(),
            embed_url: String::new(),
            duration_seconds: None,
            transcript: String::new(),
            author_email: Some(author.into()),
            author_display_name: "A".into(),
            status,
            source: InsightSource::Community,
            featured: false,
            review_note: String::new(),
            reading_minutes: 1,
            topics: vec![],
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

    fn topics() -> HashSet<String> {
        ["ai-trends".to_string(), "job-market".to_string()]
            .into_iter()
            .collect()
    }

    fn input(kind: InsightKind) -> InsightInput {
        InsightInput {
            kind: Some(kind),
            title: "How AI is reshaping hiring".into(),
            topics: vec!["ai-trends".into()],
            submit: true,
            ..Default::default()
        }
    }

    #[test]
    fn community_cannot_self_publish() {
        assert_eq!(
            status_on_save(InsightSource::Community, true),
            InsightStatus::PendingReview
        );
        assert_eq!(
            status_on_save(InsightSource::Community, false),
            InsightStatus::Draft
        );
        assert_eq!(
            status_on_save(InsightSource::Agent, false),
            InsightStatus::PendingReview
        );
        assert_eq!(
            status_on_save(InsightSource::Staff, true),
            InsightStatus::Published
        );
    }

    #[test]
    fn agents_always_go_to_review() {
        let bot = Actor {
            email: "article-writer@agents.jobshout.com".into(),
            name: "Article Writer".into(),
            is_staff: false,
            agent: true,
        };
        assert_eq!(bot.source(), InsightSource::Agent);
        assert_eq!(
            status_on_save(bot.source(), true),
            InsightStatus::PendingReview
        );
        assert_eq!(
            status_on_save(bot.source(), false),
            InsightStatus::PendingReview
        );
    }

    #[test]
    fn only_trusted_agents_publish_directly() {
        assert_eq!(
            status_on_create(InsightSource::Agent, true, true),
            InsightStatus::Published
        );
        // An untrusted agent, or a trusted one saving a draft, still goes to review.
        assert_eq!(
            status_on_create(InsightSource::Agent, true, false),
            InsightStatus::PendingReview
        );
        assert_eq!(
            status_on_create(InsightSource::Agent, false, true),
            InsightStatus::PendingReview
        );
        // Trust is for agents only; it never lifts a community author.
        assert_eq!(
            status_on_create(InsightSource::Community, true, true),
            InsightStatus::PendingReview
        );
    }

    #[test]
    fn authors_edit_own_unpublished_work_only() {
        let me = actor("me@example.com", false);
        assert!(check_can_edit(&me, &item(InsightStatus::Draft, "ME@example.com")).is_ok());
        assert!(check_can_edit(&me, &item(InsightStatus::Rejected, "me@example.com")).is_ok());
        assert!(matches!(
            check_can_edit(&me, &item(InsightStatus::Published, "me@example.com")),
            Err(DomainError::Forbidden(_))
        ));
        assert!(matches!(
            check_can_edit(&me, &item(InsightStatus::Draft, "other@example.com")),
            Err(DomainError::Forbidden(_))
        ));
        let staff = actor("ed@example.com", true);
        assert!(check_can_edit(&staff, &item(InsightStatus::Published, "x@example.com")).is_ok());
    }

    #[test]
    fn visibility() {
        let pending = item(InsightStatus::PendingReview, "me@example.com");
        assert!(!can_view(None, &pending));
        assert!(!can_view(Some(&actor("x@example.com", false)), &pending));
        assert!(can_view(Some(&actor("me@example.com", false)), &pending));
        assert!(can_view(Some(&actor("ed@example.com", true)), &pending));
        assert!(can_view(
            None,
            &item(InsightStatus::Published, "me@example.com")
        ));
    }

    #[test]
    fn moderation_transitions() {
        use InsightStatus::*;
        assert_eq!(
            moderate(PendingReview, &Moderation::Approve).unwrap(),
            Published
        );
        assert!(moderate(Draft, &Moderation::Approve).is_err());
        assert!(moderate(PendingReview, &Moderation::Reject { note: " ".into() }).is_err());
        assert_eq!(
            moderate(
                PendingReview,
                &Moderation::Reject {
                    note: "Needs sources".into()
                }
            )
            .unwrap(),
            Rejected
        );
        assert!(moderate(Published, &Moderation::Reject { note: "x".into() }).is_err());
        assert!(moderate(Draft, &Moderation::Feature(true)).is_err());
        assert_eq!(moderate(Published, &Moderation::Archive).unwrap(), Archived);
    }

    #[test]
    fn drafts_need_only_a_title() {
        let mut i = input(InsightKind::Article);
        i.submit = false;
        i.topics.clear();
        assert!(validate(InsightKind::Article, &i, &topics()).is_ok());
        i.title = "  ".into();
        assert!(validate(InsightKind::Article, &i, &topics()).is_err());
    }

    #[test]
    fn submitted_articles_need_substance() {
        let mut i = input(InsightKind::Article);
        i.body_md = "too short".into();
        assert!(validate(InsightKind::Article, &i, &topics()).is_err());
        i.body_md = "word ".repeat(MIN_LONGFORM_WORDS);
        assert!(validate(InsightKind::Article, &i, &topics()).is_ok());
        i.topics = vec!["made-up".into()];
        assert!(validate(InsightKind::Article, &i, &topics()).is_err());
    }

    #[test]
    fn posts_are_short() {
        let mut i = input(InsightKind::Post);
        i.body_md = "word ".repeat(MAX_POST_WORDS + 1);
        assert!(validate(InsightKind::Post, &i, &topics()).is_err());
    }

    #[test]
    fn media_must_fit_kind() {
        let mut i = input(InsightKind::Video);
        assert!(
            validate(InsightKind::Video, &i, &topics()).is_err(),
            "video needs media"
        );
        i.media_url = "https://open.spotify.com/episode/4rOoJ6Egrf8K2IrywzwOMk".into();
        assert!(
            validate(InsightKind::Video, &i, &topics()).is_err(),
            "spotify is audio"
        );
        i.media_url = "https://youtu.be/dQw4w9WgXcQ".into();
        let m = validate(InsightKind::Video, &i, &topics()).unwrap();
        assert_eq!(m.provider, "youtube");

        let mut p = input(InsightKind::Podcast);
        p.media_url = "https://vimeo.com/76979871".into();
        assert!(
            validate(InsightKind::Podcast, &p, &topics()).is_err(),
            "vimeo is video"
        );
        p.media_url = "https://cdn.example.com/ep.mp3".into();
        assert!(validate(InsightKind::Podcast, &p, &topics()).is_ok());
    }

    #[test]
    fn cover_needs_alt_text_on_submit() {
        let mut i = input(InsightKind::Post);
        i.body_md = "hello".into();
        i.cover_image_url = "https://example.com/c.png".into();
        assert!(validate(InsightKind::Post, &i, &topics()).is_err());
        i.cover_image_alt = "A chart of AI job postings".into();
        assert!(validate(InsightKind::Post, &i, &topics()).is_ok());
        i.cover_image_url = "javascript:alert(1)".into();
        assert!(validate(InsightKind::Post, &i, &topics()).is_err());
    }

    #[test]
    fn slugs() {
        assert_eq!(
            slugify("AI & the Job Market: 2026!"),
            "ai-the-job-market-2026"
        );
        assert_eq!(slugify("¿¿??"), "insight");
        assert!(slugify(&"a".repeat(200)).len() <= 80);
        let taken: HashSet<String> = ["ai".to_string(), "ai-2".to_string()].into_iter().collect();
        assert_eq!(next_free_slug("ai", &taken), "ai-3");
        assert_eq!(next_free_slug("ml", &taken), "ml");
    }

    #[test]
    fn emails() {
        assert!(is_email("a@b.co"));
        assert!(!is_email("a@b"));
        assert!(!is_email("a b@c.com"));
        assert!(!is_email("@c.com"));
    }

    #[test]
    fn weekly_grouping() {
        let mut a = item(InsightStatus::Published, "x@example.com");
        a.published_at = Some(Utc.with_ymd_and_hms(2026, 9, 21, 9, 0, 0).unwrap()); // Mon W39
        let mut b = item(InsightStatus::Published, "x@example.com");
        b.published_at = Some(Utc.with_ymd_and_hms(2026, 9, 24, 9, 0, 0).unwrap()); // Thu W39
        let mut c = item(InsightStatus::Published, "x@example.com");
        c.published_at = Some(Utc.with_ymd_and_hms(2026, 9, 14, 9, 0, 0).unwrap()); // W38
        let weeks = group_by_week(vec![a, c, b]);
        assert_eq!(weeks.len(), 2);
        assert_eq!(weeks[0].week, "2026-W39");
        assert_eq!(weeks[0].starts_on.to_string(), "2026-09-21");
        assert_eq!(weeks[0].items.len(), 2);
        assert!(weeks[0].items[0].published_at > weeks[0].items[1].published_at);
        assert_eq!(weeks[1].week, "2026-W38");
    }
}
