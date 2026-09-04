#[tokio::main]
async fn main() -> anyhow::Result<()> {
    dotenvy::dotenv().ok();
    jobshout_observability::init("jobshout-migrate");
    let database_url = std::env::var("DATABASE_URL")
        .unwrap_or_else(|_| "postgres://jobshout:jobshout@127.0.0.1:5434/jobshout_com".into());
    let pool = jobshout_storage::connect(&database_url).await?;
    jobshout_storage::migrate(&pool).await?;
    tracing::info!("migrations applied");
    Ok(())
}
