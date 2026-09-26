//! AI Showcase: applications built by people and AI agents.
//!
//! Everything a creator says about an app (maturity, how it was built, its
//! production evidence) is self-declared. `verification` is the only field
//! that says what JobShout itself has checked, and nothing sets it above
//! `unverified` yet.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

pub type ShowcaseAppId = Uuid;

string_enum! {
    /// What a showcase entry is. One table, like Insights: every kind shares
    /// review, stars, search, visibility and "Your apps".
    ShowcaseKind {
        App => "app",
        Agent => "agent",
        Team => "team",
    }
}

/// What an agent can do, as the directory filters by it. Stored as strings.
pub const AGENT_CAPABILITIES: &[&str] = &[
    "code_generation",
    "code_review",
    "testing",
    "debugging",
    "security",
    "deployment",
    "documentation",
    "research",
    "data_analysis",
    "design",
    "planning",
    "operations",
];

string_enum! {
    ShowcaseAppType {
        WebApplication => "web_application",
        MobileApplication => "mobile_application",
        DesktopApplication => "desktop_application",
        Api => "api",
        Saas => "saas",
        AiApplication => "ai_application",
        AgentApplication => "agent_application",
        McpServer => "mcp_server",
        DeveloperTool => "developer_tool",
        Infrastructure => "infrastructure",
        SecurityTool => "security_tool",
        DataPlatform => "data_platform",
        RagApplication => "rag_application",
        Automation => "automation",
        Workflow => "workflow",
        Game => "game",
        Marketplace => "marketplace",
        Other => "other",
    }
}

string_enum! {
    /// How far along the app is, in the creator's words.
    ShowcaseMaturity {
        Idea => "idea",
        Prototype => "prototype",
        Alpha => "alpha",
        Beta => "beta",
        ReleaseCandidate => "release_candidate",
        ProductionReady => "production_ready",
        EnterpriseReady => "enterprise_ready",
    }
}

impl ShowcaseMaturity {
    /// The two levels that must be backed by production evidence.
    pub fn needs_evidence(self) -> bool {
        matches!(self, Self::ProductionReady | Self::EnterpriseReady)
    }
}

string_enum! {
    /// Who did the building.
    ShowcaseBuildMethod {
        Human => "human",
        HumanAi => "human_ai",
        AiAssisted => "ai_assisted",
        AgentBuilt => "agent_built",
        AgentAutonomous => "agent_autonomous",
    }
}

impl ShowcaseBuildMethod {
    pub fn is_agent_built(self) -> bool {
        matches!(self, Self::AgentBuilt | Self::AgentAutonomous)
    }
}

string_enum! {
    ShowcasePricing {
        OpenSource => "open_source",
        Free => "free",
        Freemium => "freemium",
        Commercial => "commercial",
    }
}

string_enum! {
    /// Private: owner and editors only. Unlisted: anyone with the link once
    /// published, but kept out of lists and search. Public: everywhere.
    ShowcaseVisibility {
        Public => "public",
        Unlisted => "unlisted",
        Private => "private",
    }
}

string_enum! {
    ShowcaseStatus {
        Draft => "draft",
        PendingReview => "pending_review",
        Published => "published",
        Rejected => "rejected",
        Archived => "archived",
    }
}

string_enum! {
    /// What JobShout has checked. Only `unverified` is reachable today; the
    /// higher levels arrive with the verification pipeline.
    ShowcaseVerification {
        Unverified => "unverified",
        CreatorVerified => "creator_verified",
        RepositoryVerified => "repository_verified",
        BuildVerified => "build_verified",
        SecurityScanned => "security_scanned",
        ProductionVerified => "production_verified",
    }
}

/// An agent that worked on the app, as the creator describes it.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct ShowcaseAgent {
    pub name: String,
    #[serde(default)]
    pub role: String,
}

/// What the creator says backs a production claim. Self-declared.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProductionEvidence {
    #[serde(default)]
    pub automated_tests: bool,
    #[serde(default)]
    pub ci_cd: bool,
    #[serde(default)]
    pub security_scanning: bool,
    #[serde(default)]
    pub dependency_scanning: bool,
    #[serde(default)]
    pub monitoring: bool,
    #[serde(default)]
    pub backups: bool,
    #[serde(default)]
    pub documentation: bool,
    #[serde(default)]
    pub release_history: bool,
    /// Public status or uptime page, if there is one.
    #[serde(default)]
    pub status_page_url: String,
}

/// Another showcase entry this one links to: an agent that built an app, the
/// team that did, or a team's member. Only entries the viewer may see appear.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ShowcaseLink {
    pub slug: String,
    pub kind: ShowcaseKind,
    pub name: String,
    pub tagline: String,
    pub logo_url: String,
    /// What it did here ("Backend", "Reviewed the auth flow").
    pub role: String,
}

/// A link as a creator submits it: the target's slug and its role.
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct ShowcaseLinkInput {
    pub slug: String,
    #[serde(default)]
    pub role: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ShowcaseApp {
    pub id: ShowcaseAppId,
    pub kind: ShowcaseKind,
    pub slug: String,
    pub name: String,
    pub tagline: String,
    pub description_md: String,
    /// Sanitised server-side from description_md; the web renders this only.
    pub description_html: String,
    pub app_type: ShowcaseAppType,
    pub maturity: ShowcaseMaturity,
    pub build_method: ShowcaseBuildMethod,
    pub pricing: ShowcasePricing,
    pub license: String,
    pub version: String,
    pub logo_url: String,
    pub screenshots: Vec<String>,
    pub repo_url: String,
    pub demo_url: String,
    pub website_url: String,
    pub docs_url: String,
    pub technologies: Vec<String>,
    pub ai_models: Vec<String>,
    /// Agents named in free text (not in the directory).
    pub agents: Vec<ShowcaseAgent>,
    /// Where humans reviewed or approved the agents' work.
    pub human_oversight: String,
    pub evidence: ProductionEvidence,
    pub team_name: String,
    /// Agents only: who serves the model(s) in `ai_models`.
    pub model_provider: String,
    /// Agents only: tools it can call.
    pub tools: Vec<String>,
    /// Agents only: MCP servers it uses.
    pub mcp_servers: Vec<String>,
    /// Agents only: values from [`AGENT_CAPABILITIES`].
    pub capabilities: Vec<String>,
    /// Apps: directory agents that built it. Teams: members, in workflow order.
    pub linked_agents: Vec<ShowcaseLink>,
    /// Apps only: the directory team that built it.
    pub linked_team: Option<ShowcaseLink>,
    /// Agents and teams: public, published apps that link to it (directly,
    /// or through a team for agents).
    pub used_in: i64,
    /// Only returned to the creator and editors; public responses blank it.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub creator_email: Option<String>,
    pub creator_display_name: String,
    pub visibility: ShowcaseVisibility,
    pub status: ShowcaseStatus,
    pub verification: ShowcaseVerification,
    pub featured: bool,
    pub review_note: String,
    pub star_count: i32,
    /// Whether the viewer has starred it; false for anonymous viewers.
    pub starred: bool,
    pub published_at: Option<DateTime<Utc>>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

/// Create and update share one body: an edit sends the whole app again.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct ShowcaseAppInput {
    /// Required on create; an entry never changes kind.
    pub kind: Option<ShowcaseKind>,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub tagline: String,
    #[serde(default)]
    pub description_md: String,
    pub app_type: Option<ShowcaseAppType>,
    pub maturity: Option<ShowcaseMaturity>,
    pub build_method: Option<ShowcaseBuildMethod>,
    pub pricing: Option<ShowcasePricing>,
    pub visibility: Option<ShowcaseVisibility>,
    #[serde(default)]
    pub license: String,
    #[serde(default)]
    pub version: String,
    #[serde(default)]
    pub logo_url: String,
    #[serde(default)]
    pub screenshots: Vec<String>,
    #[serde(default)]
    pub repo_url: String,
    #[serde(default)]
    pub demo_url: String,
    #[serde(default)]
    pub website_url: String,
    #[serde(default)]
    pub docs_url: String,
    #[serde(default)]
    pub technologies: Vec<String>,
    #[serde(default)]
    pub ai_models: Vec<String>,
    #[serde(default)]
    pub agents: Vec<ShowcaseAgent>,
    #[serde(default)]
    pub human_oversight: String,
    #[serde(default)]
    pub evidence: ProductionEvidence,
    #[serde(default)]
    pub team_name: String,
    #[serde(default)]
    pub model_provider: String,
    #[serde(default)]
    pub tools: Vec<String>,
    #[serde(default)]
    pub mcp_servers: Vec<String>,
    #[serde(default)]
    pub capabilities: Vec<String>,
    /// Apps: directory agents that built it. Teams: members, in order.
    #[serde(default)]
    pub agent_links: Vec<ShowcaseLinkInput>,
    /// Apps only: slug of the directory team that built it, or empty.
    #[serde(default)]
    pub team_slug: String,
    /// false saves a draft; true submits (community) or publishes (editors).
    #[serde(default)]
    pub submit: bool,
}

/// A technology tag and how many public apps use it.
#[derive(Debug, Clone, Serialize)]
pub struct ShowcaseTag {
    pub name: String,
    pub count: i64,
}
