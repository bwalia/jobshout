//! Sample apps for dev and int, so the showcase is not empty on first boot.
//! Every one says it is a sample and links only to example.com, and none
//! names a real person, company or product.

use jobshout_domain::{
    ProductionEvidence, ShowcaseAgent, ShowcaseAppInput, ShowcaseAppType, ShowcaseBuildMethod,
    ShowcaseMaturity, ShowcasePricing,
};

const NOTE: &str = "_This is a sample listing seeded for development. It is not a real product._";

fn agents(list: &[(&str, &str)]) -> Vec<ShowcaseAgent> {
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
        agents: agents(s.agents),
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::validate;

    #[test]
    fn every_sample_is_valid_and_labelled() {
        for (input, _) in samples() {
            assert!(input.name.starts_with("Sample: "), "{}", input.name);
            assert!(input.description_md.contains("sample listing"));
            validate(&input).unwrap_or_else(|e| panic!("{}: {e}", input.name));
        }
    }
}
