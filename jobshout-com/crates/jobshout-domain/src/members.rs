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

string_enum! {
    /// How strongly a skill claim is backed.
    SkillEvidence {
        SelfDeclared => "self_declared",
        Endorsed => "endorsed",
        Verified => "verified",
    }
}

string_enum! {
    ConnectionStatus {
        Pending => "pending",
        Accepted => "accepted",
        Declined => "declined",
        Blocked => "blocked",
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberLink {
    pub label: String,
    pub url: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
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
    /// Whether the viewer has an accepted connection with this member.
    #[serde(default)]
    pub connected: bool,
    /// Mutual connections count.
    #[serde(default)]
    pub connection_count: i32,
    /// Public, published showcase entries and Insights.
    pub entry_count: i64,
    pub insight_count: i64,
    #[serde(default)]
    pub bio: String,
    #[serde(default)]
    pub avatar_url: String,
    #[serde(default)]
    pub location: String,
    #[serde(default)]
    pub timezone: String,
    #[serde(default)]
    pub availability: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub rate_min: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub rate_max: Option<f64>,
    #[serde(default)]
    pub rate_currency: String,
    #[serde(default)]
    pub languages: Vec<String>,
    #[serde(default)]
    pub links: Vec<MemberLink>,
    #[serde(default)]
    pub services_offered: Vec<String>,
    #[serde(default)]
    pub open_to_work: bool,
    #[serde(default)]
    pub open_to_hire: bool,
    pub created_at: DateTime<Utc>,
}

/// What a member may change about themselves. Their name comes from sign-in.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct MemberInput {
    #[serde(default)]
    pub headline: String,
    #[serde(default)]
    pub bio: Option<String>,
    #[serde(default)]
    pub avatar_url: Option<String>,
    #[serde(default)]
    pub location: Option<String>,
    #[serde(default)]
    pub timezone: Option<String>,
    #[serde(default)]
    pub availability: Option<String>,
    #[serde(default)]
    pub rate_min: Option<Option<f64>>,
    #[serde(default)]
    pub rate_max: Option<Option<f64>>,
    #[serde(default)]
    pub rate_currency: Option<String>,
    #[serde(default)]
    pub languages: Option<Vec<String>>,
    #[serde(default)]
    pub links: Option<Vec<MemberLink>>,
    #[serde(default)]
    pub services_offered: Option<Vec<String>>,
    #[serde(default)]
    pub open_to_work: Option<bool>,
    #[serde(default)]
    pub open_to_hire: Option<bool>,
    #[serde(default)]
    pub hide_from_employer: Option<bool>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberExperience {
    pub id: Uuid,
    pub title: String,
    pub organisation: String,
    pub location: String,
    pub started_on: Option<chrono::NaiveDate>,
    pub ended_on: Option<chrono::NaiveDate>,
    pub is_current: bool,
    pub summary: String,
    pub sort_order: i32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberEducation {
    pub id: Uuid,
    pub school: String,
    pub degree: String,
    pub field_of_study: String,
    pub started_on: Option<chrono::NaiveDate>,
    pub ended_on: Option<chrono::NaiveDate>,
    pub summary: String,
    pub sort_order: i32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberCertification {
    pub id: Uuid,
    pub name: String,
    pub issuer: String,
    pub issued_on: Option<chrono::NaiveDate>,
    pub expires_on: Option<chrono::NaiveDate>,
    pub credential_url: String,
    pub sort_order: i32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberPortfolioLink {
    pub id: Uuid,
    pub title: String,
    pub url: String,
    pub kind: String,
    pub showcase_slug: String,
    pub sort_order: i32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberSkill {
    pub id: Uuid,
    pub skill_name: String,
    pub evidence_level: SkillEvidence,
    pub proficiency: i32,
    pub endorsement_count: i32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberReview {
    pub id: Uuid,
    pub rating: i32,
    pub body: String,
    pub author_handle: String,
    pub author_display_name: String,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct MemberAchievement {
    pub kind: String,
    pub title: String,
    pub detail: String,
    pub earned_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Default)]
pub struct TalentSearchQuery {
    pub q: Option<String>,
    pub availability: Option<String>,
    pub open_to_work: Option<bool>,
    pub is_agent: Option<bool>,
    pub limit: i64,
    pub offset: i64,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RichMemberProfile {
    pub member: Member,
    pub experience: Vec<MemberExperience>,
    pub education: Vec<MemberEducation>,
    pub certifications: Vec<MemberCertification>,
    pub portfolio: Vec<MemberPortfolioLink>,
    pub skills: Vec<MemberSkill>,
    pub reviews: Vec<MemberReview>,
    pub achievements: Vec<MemberAchievement>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DashboardAction {
    pub id: String,
    pub title: String,
    pub reason: String,
    pub href: String,
    pub kind: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DashboardOpportunity {
    pub id: String,
    pub kind: String,
    pub title: String,
    pub summary: String,
    pub href: String,
    pub score: f64,
    pub reasons: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DashboardPipelineItem {
    pub id: String,
    pub kind: String,
    pub title: String,
    pub stage: String,
    pub next_step: String,
    pub href: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DashboardStats {
    pub profile_views_7d: i64,
    pub search_appearances_7d: i64,
    pub follower_growth_7d: i64,
    pub invitation_rate: f64,
    pub proposal_win_rate: f64,
    pub profile_strength: i32,
    pub profile_fixes: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DashboardCredits {
    pub balance: i64,
    pub expiring_soon: i64,
    pub plan: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DashboardDraft {
    pub id: String,
    pub kind: String,
    pub title: String,
    pub preview: String,
    pub status: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Dashboard {
    pub today: Vec<DashboardAction>,
    pub opportunities: Vec<DashboardOpportunity>,
    pub pipeline: Vec<DashboardPipelineItem>,
    pub stats: DashboardStats,
    pub credits: DashboardCredits,
    pub earnings_reported: f64,
    pub unread_messages: i64,
    pub unread_notifications: i64,
    pub approval_inbox: Vec<DashboardDraft>,
    pub weekly_goals: Vec<DashboardAction>,
    pub roles: Vec<String>,
    pub widgets: Vec<String>,
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
