//! Members and follows: who builds and writes what, and who keeps up with it.
//!
//! A member is the public face of an account: a handle, a name and a one-line
//! headline. The email behind it is never serialised. Platform agents (the
//! Article Writer) are members too, so people can follow them.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::{InsightKind, ShowcaseKind};

pub type MemberId = Uuid;

/// Longest headline a member may set.
pub const HEADLINE_MAX: usize = 160;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Member {
    pub id: MemberId,
    pub handle: String,
    pub display_name: String,
    pub headline: String,
    /// A platform agent (e.g. the Article Writer), not a person.
    pub is_agent: bool,
    pub follower_count: i32,
    /// Members this one follows; filled on profiles, zero in lists.
    pub following_count: i64,
    /// Whether the viewer follows this member; false for anonymous viewers.
    pub following: bool,
    /// Public, published showcase entries and Insights.
    pub entry_count: i64,
    pub insight_count: i64,
    pub created_at: DateTime<Utc>,
}

/// What a member may change about themselves. Their name comes from sign-in.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct MemberInput {
    #[serde(default)]
    pub headline: String,
}

/// The answer to follow / unfollow.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
pub struct FollowState {
    pub following: bool,
    pub follower_count: i32,
}

string_enum! {
    /// Why something is in a follower's feed: a followed member published a
    /// showcase entry (`new_entry`) or an Insight (`new_insight`); a followed
    /// entry changed after it went live (`entry_updated`); or a public app or
    /// team started using a followed agent or team (`used_in`).
    FeedItemKind {
        NewEntry => "new_entry",
        NewInsight => "new_insight",
        EntryUpdated => "entry_updated",
        UsedIn => "used_in",
    }
}

/// A showcase entry as the feed shows it.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FeedEntry {
    pub slug: String,
    pub kind: ShowcaseKind,
    pub name: String,
    pub tagline: String,
    pub logo_url: String,
    pub version: String,
}

/// An Insight as the feed shows it.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FeedInsight {
    pub slug: String,
    pub kind: InsightKind,
    pub title: String,
    pub summary: String,
}

/// Who did it, when a member did.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FeedActor {
    pub handle: String,
    pub display_name: String,
    pub is_agent: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FeedItem {
    pub kind: FeedItemKind,
    pub at: DateTime<Utc>,
    /// The member behind a new entry or Insight.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub actor: Option<FeedActor>,
    /// The entry that is new, updated, or now uses a followed one.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub entry: Option<FeedEntry>,
    /// For `used_in`: the followed agent or team it uses.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub uses: Option<FeedEntry>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub insight: Option<FeedInsight>,
}

/// A showcase entry the viewer follows, for the "Following" list.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FollowedEntry {
    #[serde(flatten)]
    pub entry: FeedEntry,
    pub follower_count: i32,
}
