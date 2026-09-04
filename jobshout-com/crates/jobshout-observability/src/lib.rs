//! Tracing bootstrap for JobShout.com services.

#![forbid(unsafe_code)]

use tracing_subscriber::EnvFilter;

/// Install a default JSON-capable subscriber. Safe to call once at process start.
pub fn init(service_name: &str) {
    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));
    tracing_subscriber::fmt()
        .with_env_filter(filter)
        .with_target(true)
        .json()
        .init();
    tracing::info!(service = service_name, "observability initialised");
}
