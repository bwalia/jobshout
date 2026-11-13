//! Outbox mailer: drains `email_outbox` via SMTP.
//!
//! Env:
//! - `DATABASE_URL`
//! - `EMAIL_OUTBOX_ENABLED` (must be truthy or the worker idles)
//! - `SMTP_HOST`, `SMTP_PORT` (default 587), `SMTP_USER`, `SMTP_PASSWORD`
//! - `SMTP_FROM` (required when sending)
//! - `WORKER_POLL_SECS` (default 15)

use std::time::Duration;

use anyhow::Context;
use jobshout_notifications::NotificationService;
use lettre::message::{Mailbox, Message};
use lettre::transport::smtp::authentication::Credentials;
use lettre::{AsyncSmtpTransport, AsyncTransport, Tokio1Executor};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    dotenvy::dotenv().ok();
    jobshout_observability::init("jobshout-worker");

    let database_url = std::env::var("DATABASE_URL")
        .unwrap_or_else(|_| "postgres://jobshout:jobshout@127.0.0.1:5434/jobshout_com".into());
    let pool = jobshout_storage::connect(&database_url).await?;
    jobshout_storage::migrate(&pool).await?;
    let notifications = NotificationService::new(pool);

    let poll = Duration::from_secs(
        std::env::var("WORKER_POLL_SECS")
            .ok()
            .and_then(|s| s.parse().ok())
            .unwrap_or(15),
    );

    if !env_flag("EMAIL_OUTBOX_ENABLED") {
        tracing::warn!("EMAIL_OUTBOX_ENABLED off — worker idle (set flag to drain outbox)");
        loop {
            tokio::time::sleep(poll).await;
        }
    }

    let mailer = build_mailer().context("configure SMTP")?;
    let from: Mailbox = std::env::var("SMTP_FROM")
        .context("SMTP_FROM is required when EMAIL_OUTBOX_ENABLED")?
        .parse()
        .context("parse SMTP_FROM")?;

    tracing::info!(?poll, "jobshout-worker draining email outbox");
    loop {
        match drain_once(&notifications, &mailer, &from).await {
            Ok(0) => {}
            Ok(n) => tracing::info!(sent = n, "outbox batch sent"),
            Err(e) => tracing::error!(error = ?e, "outbox batch failed"),
        }
        tokio::time::sleep(poll).await;
    }
}

async fn drain_once(
    notifications: &NotificationService,
    mailer: &AsyncSmtpTransport<Tokio1Executor>,
    from: &Mailbox,
) -> anyhow::Result<usize> {
    let batch = notifications.claim_outbox(20).await?;
    let mut sent = 0;
    for row in batch {
        let to: Mailbox = match row.to_email.parse() {
            Ok(m) => m,
            Err(e) => {
                let _ = notifications
                    .mark_failed(row.id, &format!("bad to_email: {e}"))
                    .await;
                continue;
            }
        };
        let builder = Message::builder()
            .from(from.clone())
            .to(to)
            .subject(row.subject.clone());
        let msg = if row.body_html.trim().is_empty() {
            builder.body(row.body_text.clone())
        } else {
            use lettre::message::{MultiPart, SinglePart};
            builder.multipart(
                MultiPart::alternative()
                    .singlepart(SinglePart::plain(row.body_text.clone()))
                    .singlepart(SinglePart::html(row.body_html.clone())),
            )
        };
        match msg {
            Ok(message) => match mailer.send(message).await {
                Ok(_) => {
                    notifications.mark_sent(row.id).await?;
                    sent += 1;
                }
                Err(e) => {
                    let _ = notifications.mark_failed(row.id, &e.to_string()).await;
                }
            },
            Err(e) => {
                let _ = notifications.mark_failed(row.id, &e.to_string()).await;
            }
        }
    }
    Ok(sent)
}

fn build_mailer() -> anyhow::Result<AsyncSmtpTransport<Tokio1Executor>> {
    let host = std::env::var("SMTP_HOST").context("SMTP_HOST is required")?;
    let port: u16 = std::env::var("SMTP_PORT")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(587);
    let mut builder = AsyncSmtpTransport::<Tokio1Executor>::starttls_relay(&host)?.port(port);
    let user = std::env::var("SMTP_USER").unwrap_or_default();
    let pass = std::env::var("SMTP_PASSWORD").unwrap_or_default();
    if !user.is_empty() {
        builder = builder.credentials(Credentials::new(user, pass));
    }
    Ok(builder.build())
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
