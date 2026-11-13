//! Lightweight scheduler: periodic maintenance ticks.
//!
//! Phase 0: re-queue stale `sending` outbox rows and emit a heartbeat log.
//! Later phases add project close, credit expiry, digest rolls, etc.
//!
//! Env: `DATABASE_URL`, `SCHEDULER_TICK_SECS` (default 60), `SCHEDULER_ENABLED`.

use std::time::Duration;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    dotenvy::dotenv().ok();
    jobshout_observability::init("jobshout-scheduler");

    let tick = Duration::from_secs(
        std::env::var("SCHEDULER_TICK_SECS")
            .ok()
            .and_then(|s| s.parse().ok())
            .unwrap_or(60),
    );

    if !env_flag("SCHEDULER_ENABLED") {
        tracing::warn!("SCHEDULER_ENABLED off — scheduler idle");
        loop {
            tokio::time::sleep(tick).await;
        }
    }

    let database_url = std::env::var("DATABASE_URL")
        .unwrap_or_else(|_| "postgres://jobshout:jobshout@127.0.0.1:5434/jobshout_com".into());
    let pool = jobshout_storage::connect(&database_url).await?;
    jobshout_storage::migrate(&pool).await?;

    tracing::info!(?tick, "jobshout-scheduler running");
    loop {
        match tick_once(&pool).await {
            Ok(()) => {}
            Err(e) => tracing::error!(error = ?e, "scheduler tick failed"),
        }
        tokio::time::sleep(tick).await;
    }
}

async fn tick_once(pool: &sqlx::PgPool) -> anyhow::Result<()> {
    // Rows stuck in `sending` for > 10 minutes go back to retry.
    let n = sqlx::query(
        r#"
        UPDATE email_outbox
        SET status = 'retry',
            last_error = 'stale sending recovered by scheduler',
            scheduled_at = NOW(),
            updated_at = NOW()
        WHERE status = 'sending'
          AND updated_at < NOW() - INTERVAL '10 minutes'
        "#,
    )
    .execute(pool)
    .await?
    .rows_affected();
    if n > 0 {
        tracing::info!(recovered = n, "recovered stale outbox rows");
    }
    tracing::debug!(at = %chrono::Utc::now(), "scheduler heartbeat");
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
