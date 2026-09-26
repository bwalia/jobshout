use anyhow::Context;
use jobshout_api::{router, AppState};
use jobshout_applications::ApplicationService;
use jobshout_candidates::CandidateService;
use jobshout_content::InsightService;
use jobshout_jobs::{seed_organisation_id, JobService};
use jobshout_showcase::ShowcaseService;
use std::net::SocketAddr;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    dotenvy::dotenv().ok();
    jobshout_observability::init("jobshout-api");

    let database_url = std::env::var("DATABASE_URL")
        .unwrap_or_else(|_| "postgres://jobshout:jobshout@127.0.0.1:5434/jobshout_com".into());
    let host = std::env::var("HOST").unwrap_or_else(|_| "0.0.0.0".into());
    let port: u16 = std::env::var("PORT")
        .ok()
        .and_then(|p| p.parse().ok())
        .unwrap_or(8088);

    let pool = jobshout_storage::connect(&database_url).await?;
    jobshout_storage::migrate(&pool).await?;

    let staff_emails = std::env::var("INSIGHTS_STAFF_EMAILS").unwrap_or_default();
    let insights = InsightService::new(pool.clone(), staff_emails.split(',').map(str::to_string));
    if env_flag("INSIGHTS_SEED_SAMPLES") {
        let n = insights
            .seed_samples()
            .await
            .context("seed sample insights")?;
        if n > 0 {
            tracing::info!(count = n, "seeded sample insights");
        }
    }
    let showcase = ShowcaseService::new(pool.clone());
    if env_flag("SHOWCASE_SEED_SAMPLES") {
        let n = showcase
            .seed_samples()
            .await
            .context("seed sample showcase apps")?;
        if n > 0 {
            tracing::info!(count = n, "seeded sample showcase apps");
        }
    }
    let site_url = std::env::var("PUBLIC_SITE_URL")
        .ok()
        .filter(|s| !s.trim().is_empty())
        .unwrap_or_else(|| "https://jobshout.com".into());
    // The API is reachable through the public gateway, so a bare identity
    // header proves nothing. Without the web tier's shared token, refuse to
    // start unless someone explicitly asked for unsigned headers (local dev).
    let internal_token = std::env::var("JOBSHOUT_INTERNAL_TOKEN")
        .ok()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty());
    if internal_token.is_none() {
        if env_flag("JOBSHOUT_ALLOW_UNSIGNED_IDENTITY") {
            tracing::warn!(
                "JOBSHOUT_INTERNAL_TOKEN unset: trusting unsigned identity headers (dev only)"
            );
        } else {
            anyhow::bail!(
                "JOBSHOUT_INTERNAL_TOKEN is required (set JOBSHOUT_ALLOW_UNSIGNED_IDENTITY=1 for local dev)"
            );
        }
    }

    let state = AppState {
        jobs: JobService::new(pool.clone(), seed_organisation_id()),
        candidates: CandidateService::new(pool.clone()),
        applications: ApplicationService::new(pool),
        insights,
        showcase,
        site_url: site_url.trim_end_matches('/').into(),
        internal_token: internal_token.map(Into::into),
    };

    let app = router(state);
    let addr: SocketAddr = format!("{host}:{port}")
        .parse()
        .context("parse bind address")?;
    tracing::info!(%addr, "jobshout-com API listening");
    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;
    Ok(())
}

fn env_flag(name: &str) -> bool {
    matches!(
        std::env::var(name)
            .unwrap_or_default()
            .trim()
            .to_ascii_lowercase()
            .as_str(),
        "1" | "true" | "yes"
    )
}
