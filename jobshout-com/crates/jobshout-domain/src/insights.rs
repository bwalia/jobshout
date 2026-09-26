//! Insights: the news, articles, blogs, podcasts and videos hub.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

pub type InsightId = Uuid;

string_enum! {
    /// What the item is. Every kind shares one table, feed and review queue.
    InsightKind {
        Post => "post",
        Article => "article",
        Blog => "blog",
        Podcast => "podcast",
        Video => "video",
    }
}

string_enum! {
    InsightStatus {
        Draft => "draft",
        PendingReview => "pending_review",
        Published => "published",
        Rejected => "rejected",
        Archived => "archived",
    }
}

string_enum! {
    /// Who wrote it. Agent submissions always go through review.
    InsightSource {
        Staff => "staff",
        Community => "community",
        Agent => "agent",
    }
}

string_enum! {
    NewsletterStatus {
        Pending => "pending",
        Confirmed => "confirmed",
        Unsubscribed => "unsubscribed",
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InsightTopic {
    pub slug: String,
    pub name: String,
    pub description: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Insight {
    pub id: InsightId,
    pub kind: InsightKind,
    pub slug: String,
    pub title: String,
    pub summary: String,
    pub body_md: String,
    pub body_html: String,
    pub cover_image_url: String,
    pub cover_image_alt: String,
    pub link_url: String,
    pub media_url: String,
    /// youtube | vimeo | spotify | apple | file, or empty when there is no media.
    pub embed_provider: String,
    /// The iframe src (or the file URL for `file`), derived from `media_url`.
    pub embed_url: String,
    pub duration_seconds: Option<i32>,
    pub transcript: String,
    /// Only returned to the author and staff; public responses blank it.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub author_email: Option<String>,
    pub author_display_name: String,
    pub status: InsightStatus,
    pub source: InsightSource,
    pub featured: bool,
    pub review_note: String,
    pub reading_minutes: i32,
    pub topics: Vec<InsightTopic>,
    pub published_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Create and update share one body: an edit sends the whole item again.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct InsightInput {
    pub kind: Option<InsightKind>,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub summary: String,
    #[serde(default)]
    pub body_md: String,
    #[serde(default)]
    pub cover_image_url: String,
    #[serde(default)]
    pub cover_image_alt: String,
    #[serde(default)]
    pub link_url: String,
    #[serde(default)]
    pub media_url: String,
    pub duration_seconds: Option<i32>,
    #[serde(default)]
    pub transcript: String,
    #[serde(default)]
    pub topics: Vec<String>,
    /// false saves a draft; true submits (community) or publishes (staff).
    #[serde(default)]
    pub submit: bool,
}

#[derive(Debug, Clone, Serialize)]
pub struct NewsletterDigest {
    /// ISO week label, e.g. "2026-W39".
    pub week: String,
    pub starts_on: chrono::NaiveDate,
    pub items: Vec<Insight>,
}
