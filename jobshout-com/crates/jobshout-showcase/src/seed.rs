//! Sample apps for dev and int, so the showcase is not empty on first boot.
//! Every one says it is a sample and links only to example.com, and none
//! names a real person, company or product.

use jobshout_domain::{
    ProductionEvidence, ShowcaseAgent, ShowcaseAppInput, ShowcaseAppType, ShowcaseBuildMethod,
    ShowcaseKind, ShowcaseLinkInput, ShowcaseMaturity, ShowcasePricing,
};

/// Who the samples belong to; linking only touches apps this account owns.
pub const EDITOR: &str = "editors@jobshout.com";

const NOTE: &str = "_This is a sample listing seeded for development. It is not a real product._";

fn named_agents(list: &[(&str, &str)]) -> Vec<ShowcaseAgent> {
    list.iter()
        .map(|(name, role)| ShowcaseAgent {
            name: name.to_string(),
            role: role.to_string(),
        })
        .collect()
}

fn tags(list: &[&str]) -> Vec<String> {
    list.iter().map(|t| t.to_string()).collect()
}

struct Sample<'a> {
    name: &'a str,
    tagline: &'a str,
    body: &'a str,
    app_type: ShowcaseAppType,
    maturity: ShowcaseMaturity,
    build: ShowcaseBuildMethod,
    pricing: ShowcasePricing,
    technologies: &'a [&'a str],
    models: &'a [&'a str],
    agents: &'a [(&'a str, &'a str)],
}

fn app(s: Sample) -> ShowcaseAppInput {
    let slug = s.name.to_ascii_lowercase().replace(' ', "-");
    ShowcaseAppInput {
        name: format!("Sample: {}", s.name),
        tagline: s.tagline.into(),
        description_md: format!("{}\n\n{NOTE}", s.body),
        app_type: Some(s.app_type),
        maturity: Some(s.maturity),
        build_method: Some(s.build),
        pricing: Some(s.pricing),
        license: if s.pricing == ShowcasePricing::OpenSource {
            "MIT".into()
        } else {
            String::new()
        },
        version: "0.1.0".into(),
        repo_url: format!("https://example.com/{slug}/source"),
        demo_url: format!("https://example.com/{slug}/demo"),
        technologies: tags(s.technologies),
        ai_models: tags(s.models),
        agents: named_agents(s.agents),
        human_oversight:
            "A maintainer reviews every change before it is merged and approves each release."
                .into(),
        submit: true,
        ..Default::default()
    }
}

/// (input, featured)
pub fn samples() -> Vec<(ShowcaseAppInput, bool)> {
    let mut vector_store = app(Sample {
        name: "Vector Notebook",
        tagline: "Sample app. A searchable notebook that answers questions from your own documents.",
        body: "## What it does\n\nDrop in documents and ask questions about them. Answers quote the passage they came from, so you can check them.\n\n## How it was built\n\nAn architecture agent proposed the retrieval pipeline, a backend agent wrote the ingestion service, and a testing agent built the evaluation set. A maintainer reviewed every pull request.",
        app_type: ShowcaseAppType::RagApplication,
        maturity: ShowcaseMaturity::ProductionReady,
        build: ShowcaseBuildMethod::AgentBuilt,
        pricing: ShowcasePricing::OpenSource,
        technologies: &["Rust", "PostgreSQL", "pgvector", "Next.js"],
        models: &["Claude"],
        agents: &[
            ("Architecture Agent", "Designed the retrieval pipeline"),
            ("Backend Agent", "Wrote the ingestion service"),
            ("Testing Agent", "Built the evaluation set"),
        ],
    });
    vector_store.evidence = ProductionEvidence {
        automated_tests: true,
        ci_cd: true,
        security_scanning: true,
        dependency_scanning: true,
        monitoring: true,
        documentation: true,
        release_history: true,
        ..Default::default()
    };

    let mcp = app(Sample {
        name: "Calendar MCP Server",
        tagline: "Sample app. An MCP server that lets agents read and propose calendar changes.",
        body: "## What it does\n\nExposes calendar lookups and change proposals as MCP tools. Every write is a proposal that a person accepts.\n\n## How it was built\n\nWritten with an AI pair programmer; the human author wrote the permission model by hand.",
        app_type: ShowcaseAppType::McpServer,
        maturity: ShowcaseMaturity::Beta,
        build: ShowcaseBuildMethod::HumanAi,
        pricing: ShowcasePricing::OpenSource,
        technologies: &["TypeScript", "MCP", "Node.js"],
        models: &["Claude"],
        agents: &[],
    });

    let ops = app(Sample {
        name: "Incident Scribe",
        tagline: "Sample app. Turns an incident channel into a timeline and a draft postmortem.",
        body: "## What it does\n\nReads an incident channel, builds a timeline, and drafts the postmortem for the on-call engineer to edit.\n\n## How it was built\n\nA team of agents built it end to end: a product agent wrote the brief, frontend and backend agents built it, and a security agent reviewed the data handling. A human approved each release.",
        app_type: ShowcaseAppType::DeveloperTool,
        maturity: ShowcaseMaturity::Alpha,
        build: ShowcaseBuildMethod::AgentAutonomous,
        pricing: ShowcasePricing::Freemium,
        technologies: &["Go", "React", "Kubernetes"],
        models: &["Claude", "Open-weight model"],
        agents: &[
            ("Product Agent", "Wrote the brief"),
            ("Frontend Agent", "Built the timeline UI"),
            ("Backend Agent", "Built the channel reader"),
            ("Security Agent", "Reviewed data handling"),
        ],
    });

    let game = app(Sample {
        name: "Word Garden",
        tagline: "Sample app. A small daily word game, built in a weekend with AI assistance.",
        body: "## What it does\n\nOne puzzle a day: grow a garden by finding words that share letters with yesterday's.\n\n## How it was built\n\nThe author designed the game and used an AI assistant for the scoring code and the tests.",
        app_type: ShowcaseAppType::Game,
        maturity: ShowcaseMaturity::Prototype,
        build: ShowcaseBuildMethod::AiAssisted,
        pricing: ShowcasePricing::Free,
        technologies: &["TypeScript", "Svelte"],
        models: &["Claude"],
        agents: &[],
    });

    vec![
        (vector_store, true),
        (mcp, false),
        (ops, false),
        (game, false),
    ]
}

struct AgentSample<'a> {
    name: &'a str,
    tagline: &'a str,
    body: &'a str,
    provider: &'a str,
    models: &'a [&'a str],
    skills: &'a [&'a str],
    tools: &'a [&'a str],
    mcp: &'a [&'a str],
    capabilities: &'a [&'a str],
}

fn agent(s: AgentSample) -> ShowcaseAppInput {
    ShowcaseAppInput {
        kind: Some(ShowcaseKind::Agent),
        name: format!("Sample: {}", s.name),
        tagline: s.tagline.into(),
        description_md: format!("{}\n\n{NOTE}", s.body),
        pricing: Some(ShowcasePricing::OpenSource),
        license: "MIT".into(),
        version: "0.1.0".into(),
        repo_url: format!(
            "https://example.com/{}/source",
            s.name.to_ascii_lowercase().replace(' ', "-")
        ),
        model_provider: s.provider.into(),
        ai_models: tags(s.models),
        technologies: tags(s.skills),
        tools: tags(s.tools),
        mcp_servers: tags(s.mcp),
        capabilities: tags(s.capabilities),
        submit: true,
        ..Default::default()
    }
}

/// Sample agents. Slugs are "sample-<name>", which the team and app links use.
pub fn agents() -> Vec<ShowcaseAppInput> {
    vec![
        agent(AgentSample {
            name: "Architecture Agent",
            tagline: "Sample agent. Turns a product brief into a service design and a build plan.",
            body: "## What it does\n\nReads a brief, proposes the services, data model and interfaces, and writes a build plan other agents can pick up.\n\n## Where people come in\n\nA maintainer approves the design before any code is written.",
            provider: "Anthropic",
            models: &["Claude"],
            skills: &["System design", "APIs", "PostgreSQL"],
            tools: &["GitHub"],
            mcp: &["Filesystem"],
            capabilities: &["planning", "design"],
        }),
        agent(AgentSample {
            name: "Rust Backend Agent",
            tagline: "Sample agent. Writes Axum services with tests, from a design it is given.",
            body: "## What it does\n\nImplements endpoints, persistence and tests in Rust, and opens a pull request per change.\n\n## Where people come in\n\nEvery pull request is reviewed by a person before it merges.",
            provider: "Anthropic",
            models: &["Claude"],
            skills: &["Rust", "Axum", "Tokio", "SQLx"],
            tools: &["GitHub", "Docker"],
            mcp: &["GitHub"],
            capabilities: &["code_generation", "testing", "debugging"],
        }),
        agent(AgentSample {
            name: "Security Review Agent",
            tagline: "Sample agent. Reviews changes for injection, auth and secrets problems.",
            body: "## What it does\n\nReads each pull request, flags risky patterns with the line and the reason, and suggests a fix.\n\n## Where people come in\n\nFindings are advice: a person decides what to change.",
            provider: "Anthropic",
            models: &["Claude"],
            skills: &["Application security", "Kubernetes"],
            tools: &["GitHub", "Semgrep"],
            mcp: &[],
            capabilities: &["security", "code_review"],
        }),
        agent(AgentSample {
            name: "Docs Agent",
            tagline: "Sample agent. Keeps READMEs and API docs in step with the code.",
            body: "## What it does\n\nWatches merged changes and proposes documentation updates, including examples that are run to check they still work.\n\n## Where people come in\n\nA maintainer merges the proposed docs.",
            provider: "Open-weight model",
            models: &["Open-weight model"],
            skills: &["Technical writing", "OpenAPI"],
            tools: &["GitHub"],
            mcp: &["Filesystem"],
            capabilities: &["documentation"],
        }),
    ]
}

fn link(slug: &str, role: &str) -> ShowcaseLinkInput {
    ShowcaseLinkInput {
        slug: slug.into(),
        role: role.into(),
    }
}

pub fn teams() -> Vec<ShowcaseAppInput> {
    vec![ShowcaseAppInput {
        kind: Some(ShowcaseKind::Team),
        name: "Sample: Service Factory".into(),
        tagline: "Sample team. Design, build, review and document a backend service.".into(),
        description_md: format!(
            "## How the team works\n\nThe architecture agent writes the plan, the backend agent builds it, the security agent reviews every change, and the docs agent writes it up. A person approves the plan and every merge.\n\n{NOTE}"
        ),
        agent_links: vec![
            link("sample-architecture-agent", "Plans the service"),
            link("sample-rust-backend-agent", "Builds it"),
            link("sample-security-review-agent", "Reviews every change"),
            link("sample-docs-agent", "Documents it"),
        ],
        submit: true,
        ..Default::default()
    }]
}

/// (sample entry slug, how many open jobs to link) for int.
pub fn hiring() -> Vec<(&'static str, usize)> {
    vec![("sample-vector-notebook", 2), ("sample-service-factory", 1)]
}

/// (sample app slug, links to set on it). The team slug is linked as the team.
pub fn app_links() -> Vec<(&'static str, Vec<(&'static str, &'static str)>)> {
    vec![
        (
            "sample-vector-notebook",
            vec![
                (
                    "sample-architecture-agent",
                    "Designed the retrieval pipeline",
                ),
                ("sample-rust-backend-agent", "Wrote the ingestion service"),
            ],
        ),
        (
            "sample-incident-scribe",
            vec![("sample-service-factory", "")],
        ),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::validate;

    #[test]
    fn every_sample_is_valid_and_labelled() {
        let all = samples()
            .into_iter()
            .map(|(i, _)| i)
            .chain(agents())
            .chain(teams());
        for input in all {
            assert!(input.name.starts_with("Sample: "), "{}", input.name);
            assert!(input.description_md.contains("sample listing"));
            let kind = input.kind.unwrap_or(ShowcaseKind::App);
            validate(kind, &input).unwrap_or_else(|e| panic!("{}: {e}", input.name));
        }
    }

    #[test]
    fn team_and_app_links_name_sample_agents() {
        use jobshout_content::rules::slugify;
        let agent_slugs: Vec<String> = agents().iter().map(|a| slugify(&a.name)).collect();
        for team in teams() {
            for l in &team.agent_links {
                assert!(agent_slugs.contains(&l.slug), "{}", l.slug);
            }
        }
        let team_slugs: Vec<String> = teams().iter().map(|t| slugify(&t.name)).collect();
        for (_, links) in app_links() {
            for (slug, _) in links {
                let s = slug.to_string();
                assert!(
                    agent_slugs.contains(&s) || team_slugs.contains(&s),
                    "{slug}"
                );
            }
        }
    }
}
