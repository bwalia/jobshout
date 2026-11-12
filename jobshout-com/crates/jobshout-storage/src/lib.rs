//! PostgreSQL pool helpers and optional MinIO signed uploads.

#![forbid(unsafe_code)]

pub mod uploads;

use anyhow::{Context, Result};
use sqlx::postgres::PgPoolOptions;
use sqlx::PgPool;

pub use uploads::{SignedUpload, UploadConfig};

pub async fn connect(database_url: &str) -> Result<PgPool> {
    let pool = PgPoolOptions::new()
        .max_connections(10)
        .connect(database_url)
        .await
        .context("connect to postgres")?;
    Ok(pool)
}

pub async fn migrate(pool: &PgPool) -> Result<()> {
    sqlx::migrate!("../../migrations")
        .run(pool)
        .await
        .context("run sqlx migrations")?;
    Ok(())
}
