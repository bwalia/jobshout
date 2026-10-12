//! Member rules: pure, so they are unit-tested without a database.

use std::collections::HashSet;

use jobshout_content::rules::{next_free_slug, slugify};
use jobshout_domain::{DomainError, FeedItem, FeedItemKind, HEADLINE_MAX};

/// Handles the web routes under `/members` already use.
const RESERVED: &[&str] = &["me", "new", "edit", "following", "settings", "admin"];

/// The name shown for a member. Sign-in may give none; the email is never a
/// fallback, since the member page is public.
pub fn display_name(name: &str) -> String {
    let name = name.split_whitespace().collect::<Vec<_>>().join(" ");
    if name.is_empty() {
        "JobShout member".into()
    } else {
        name.chars().take(80).collect()
    }
}

/// The handle a name asks for, before collisions: the name slugified, and
/// never anything derived from the email.
pub fn handle_base(name: &str) -> String {
    // slugify keeps ASCII letters and digits only; with none it has nothing
    // to work from (and falls back to its own default).
    if !name.chars().any(|c| c.is_ascii_alphanumeric()) {
        return "member".into();
    }
    let s: String = slugify(name).chars().take(40).collect();
    let s = s.trim_matches('-').to_string();
    if RESERVED.contains(&s.as_str()) {
        format!("{s}-member")
    } else {
        s
    }
}

/// First handle for `base` not in `taken`.
pub fn free_handle(base: &str, taken: &HashSet<String>) -> String {
    next_free_slug(base, taken)
}

/// Platform agents act under `{name}@agents.jobshout.com` (see
/// `jobshout_content::InsightService::agent_actor`).
pub fn is_agent_email(email: &str) -> bool {
    email
        .trim()
        .to_ascii_lowercase()
        .ends_with("@agents.jobshout.com")
}

pub fn clean_headline(raw: &str) -> Result<String, DomainError> {
    let s = raw.split_whitespace().collect::<Vec<_>>().join(" ");
    if s.chars().count() > HEADLINE_MAX {
        return Err(DomainError::Validation(format!(
            "keep your headline to {HEADLINE_MAX} characters"
        )));
    }
    Ok(s)
}

/// One line per thing that happened. An app a followed member published
/// that also uses a followed agent is one story ("new"), not two.
pub fn dedupe_feed(items: Vec<FeedItem>) -> Vec<FeedItem> {
    let new_entries: HashSet<String> = items
        .iter()
        .filter(|i| i.kind == FeedItemKind::NewEntry)
        .filter_map(|i| i.entry.as_ref().map(|e| e.slug.clone()))
        .collect();
    let mut seen = HashSet::new();
    items
        .into_iter()
        .filter(|i| {
            let slug = i
                .entry
                .as_ref()
                .map(|e| e.slug.as_str())
                .or(i.insight.as_ref().map(|x| x.slug.as_str()))
                .unwrap_or_default();
            if i.kind == FeedItemKind::UsedIn && new_entries.contains(slug) {
                return false;
            }
            let uses = i.uses.as_ref().map(|u| u.slug.as_str()).unwrap_or_default();
            seen.insert(format!("{}|{slug}|{uses}", i.kind.as_str()))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;
    use jobshout_domain::{FeedEntry, ShowcaseKind};

    fn entry(slug: &str) -> FeedEntry {
        FeedEntry {
            slug: slug.into(),
            kind: ShowcaseKind::App,
            name: slug.into(),
            tagline: String::new(),
            logo_url: String::new(),
            version: String::new(),
        }
    }

    fn item(kind: FeedItemKind, slug: &str, uses: Option<&str>) -> FeedItem {
        FeedItem {
            kind,
            at: Utc::now(),
            actor: None,
            entry: Some(entry(slug)),
            uses: uses.map(entry),
            insight: None,
        }
    }

    #[test]
    fn handles_come_from_the_name_not_the_email() {
        assert_eq!(handle_base("Ada Lovelace"), "ada-lovelace");
        assert_eq!(handle_base("  "), "member");
        assert_eq!(
            handle_base("JobShout Article Writer"),
            "jobshout-article-writer"
        );
        assert_eq!(display_name(""), "JobShout member");
        assert_eq!(display_name("  Ada   Lovelace "), "Ada Lovelace");
    }

    #[test]
    fn reserved_handles_are_never_given_out() {
        assert_eq!(handle_base("Me"), "me-member");
        assert_eq!(handle_base("Following"), "following-member");
    }

    #[test]
    fn handles_are_bounded_and_unique() {
        let long = "a".repeat(200);
        assert_eq!(handle_base(&long).len(), 40);
        let taken: HashSet<String> = ["ada".to_string(), "ada-2".to_string()].into();
        assert_eq!(free_handle("ada", &taken), "ada-3");
    }

    #[test]
    fn agent_addresses_are_recognised() {
        assert!(is_agent_email("article-writer@agents.jobshout.com"));
        assert!(!is_agent_email("ada@example.com"));
    }

    #[test]
    fn headlines_are_trimmed_and_capped() {
        assert_eq!(
            clean_headline("  Builds   agents ").unwrap(),
            "Builds agents"
        );
        assert!(clean_headline(&"x".repeat(HEADLINE_MAX + 1)).is_err());
    }

    #[test]
    fn a_new_app_that_uses_a_followed_agent_is_one_story() {
        let out = dedupe_feed(vec![
            item(FeedItemKind::NewEntry, "app-a", None),
            item(FeedItemKind::UsedIn, "app-a", Some("agent-x")),
            item(FeedItemKind::UsedIn, "app-b", Some("agent-x")),
            item(FeedItemKind::UsedIn, "app-b", Some("agent-x")),
            item(FeedItemKind::UsedIn, "app-b", Some("agent-y")),
        ]);
        let kinds: Vec<_> = out
            .iter()
            .map(|i| {
                (
                    i.kind,
                    i.entry.as_ref().unwrap().slug.clone(),
                    i.uses.as_ref().map(|u| u.slug.clone()),
                )
            })
            .collect();
        assert_eq!(
            kinds,
            vec![
                (FeedItemKind::NewEntry, "app-a".into(), None),
                (FeedItemKind::UsedIn, "app-b".into(), Some("agent-x".into())),
                (FeedItemKind::UsedIn, "app-b".into(), Some("agent-y".into())),
            ]
        );
    }
}
