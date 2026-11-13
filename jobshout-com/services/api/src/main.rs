use anyhow::Context;
use jobshout_api::{router, AppState};
use jobshout_applications::ApplicationService;
use jobshout_auth::AccountService;
use jobshout_billing::BillingService;
use jobshout_candidates::CandidateService;
use jobshout_content::InsightService;
use jobshout_events::EventService;
use jobshout_freelance::FreelanceService;
use jobshout_groups::GroupsService;
use jobshout_hire::HireService;
use jobshout_integrations::PlatformClient;
use jobshout_jobs::{seed_organisation_id, JobService};
use jobshout_messaging::MessagingService;
use jobshout_notifications::NotificationService;
use jobshout_showcase::ShowcaseService;
use jobshout_users::MemberService;
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
    // Agents whose submissions publish without review — only those whose
    // platform has a person approve each item before sending it.
    let trusted_agents = std::env::var("INSIGHTS_TRUSTED_AGENTS").unwrap_or_default();
    let accounts = AccountService::new(pool.clone(), staff_emails.split(',').map(str::to_string));
    match accounts.sync_bootstrap_staff().await {
        Ok(0) => {}
        Ok(n) => tracing::info!(count = n, "synced bootstrap staff roles"),
        Err(e) => tracing::warn!(error = ?e, "could not sync bootstrap staff"),
    }
    let insights = InsightService::new(pool.clone(), staff_emails.split(',').map(str::to_string))
        .with_trusted_agents(trusted_agents.split(',').map(str::to_string));
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
    // After seeding, so sample creators get profiles too. Profiles are a
    // nicety: a failure here must not keep the site down.
    let members = MemberService::new(pool.clone());
    match members.backfill().await {
        Ok(0) => {}
        Ok(n) => tracing::info!(count = n, "created members for existing creators"),
        Err(e) => tracing::warn!(error = ?e, "could not backfill members"),
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

    let email_outbox_enabled = env_flag("EMAIL_OUTBOX_ENABLED");
    let members_rich_enabled = env_flag("MEMBERS_RICH_ENABLED");
    let dashboard_enabled = env_flag("DASHBOARD_ENABLED");
    let messaging_enabled = env_flag("MESSAGING_ENABLED");
    let projects_live = env_flag("PROJECTS_LIVE");
    let credits_enabled = env_flag("CREDITS_ENABLED");
    let purchases_enabled = env_flag("PURCHASES_ENABLED");
    let ai_features = env_flag("AI_FEATURES");
    let hire_enabled = env_flag("HIRE_ENABLED");
    let groups_enabled = env_flag("GROUPS_ENABLED");
    let freelance = FreelanceService::new(pool.clone());
    let hire = HireService::new(pool.clone());
    let groups = GroupsService::new(pool.clone());
    let billing = BillingService::new(pool.clone());
    let platform = PlatformClient::from_env();
    if ai_features && platform.is_none() {
        tracing::warn!("AI_FEATURES on but JOBSHOUT_PLATFORM_URL/API_KEY unset");
    }
    if projects_live && env_flag("PROJECTS_SEED_SAMPLES") {
        // Seed under a synthetic owner account so samples have an owner_account_id.
        match accounts
            .ensure("projects-samples@jobshout.local", "Sample Projects", true)
            .await
        {
            Ok(owner) => match freelance.seed_samples(owner.id).await {
                Ok(0) => {}
                Ok(n) => tracing::info!(count = n, "seeded sample projects"),
                Err(e) => tracing::warn!(error = ?e, "could not seed sample projects"),
            },
            Err(e) => tracing::warn!(error = ?e, "could not ensure sample projects owner"),
        }
    }
    let state = AppState {
        jobs: JobService::new(pool.clone(), seed_organisation_id()),
        candidates: CandidateService::new(pool.clone()),
        applications: ApplicationService::new(pool.clone()),
        insights,
        showcase,
        members,
        accounts,
        events: EventService::new(pool.clone()),
        notifications: NotificationService::new(pool.clone()),
        messaging: MessagingService::new(pool.clone()),
        freelance,
        hire,
        groups,
        billing,
        ai_pool: Some(pool.clone()),
        platform,
        site_url: site_url.trim_end_matches('/').into(),
        internal_token: internal_token.map(Into::into),
        email_outbox_enabled,
        members_rich_enabled,
        dashboard_enabled,
        messaging_enabled,
        projects_live,
        credits_enabled,
        purchases_enabled,
        ai_features,
        hire_enabled,
        groups_enabled,
    };

    let app = router(state);
    let addr: SocketAddr = format!("{host}:{port}")
        .parse()
        .context("parse bind address")?;
    tracing::info!(%addr, email_outbox_enabled, "jobshout-com API listening");
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
