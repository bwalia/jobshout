use anyhow::Context;
use jobshout_api::{router, AppState};
use jobshout_jobs::{seed_organisation_id, JobService};
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

    let state = AppState {
        jobs: JobService::new(pool, seed_organisation_id()),
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
