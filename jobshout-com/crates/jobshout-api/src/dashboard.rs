//! Dashboard command centre — Phase 1. Gated by `DASHBOARD_ENABLED`.

use axum::extract::State;
use axum::http::HeaderMap;
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::{Dashboard, DomainError};
use serde::Deserialize;

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new().route("/api/v1/dashboard", get(get_dashboard).put(put_widgets))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_enabled(state: &AppState) -> Result<(), ApiError> {
    if state.dashboard_enabled {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

async fn get_dashboard(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<Dashboard>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let mut dash = state.members.dashboard(&who).await.map_err(err)?;
    if state.messaging_enabled {
        if let Ok(me) = state.members.ensure(&who).await {
            dash.unread_messages = state.messaging.unread_count(me.id).await.unwrap_or(0);
        }
    }
    if let Ok(account) = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
    {
        dash.unread_notifications = state
            .notifications
            .unread_count(account.id)
            .await
            .unwrap_or(0);
        // Real balance when credits are on; an empty plan tells the page to
        // hide the tile instead of showing a made-up zero.
        dash.credits = match state.credits_enabled {
            true => match state.billing.balance(account.id).await {
                Ok(b) => jobshout_domain::DashboardCredits {
                    balance: i64::from(b.balance),
                    expiring_soon: i64::from(b.expiring_soon),
                    plan: b.plan,
                },
                Err(_) => empty_credits(),
            },
            false => empty_credits(),
        };
    }
    // Opportunities are the member's real top job matches (same ranking as
    // /profile/matches), only when they have a candidate profile.
    if let Ok(profile) = state.candidates.get_by_email(&who.email).await {
        if let Ok(jobs) = state
            .jobs
            .list(100, 0, Some(jobshout_domain::JobStatus::Published))
            .await
        {
            dash.opportunities = jobshout_matching::rank_jobs(&profile, &jobs, 5)
                .into_iter()
                .filter(|m| m.score > 0)
                .map(|m| jobshout_domain::DashboardOpportunity {
                    id: m.job.id.to_string(),
                    kind: "job".into(),
                    title: m.job.title.clone(),
                    summary: m.job.summary.clone(),
                    href: format!("/jobs/{}", m.job.id),
                    score: f64::from(m.score) / 100.0,
                    reasons: m.reasons,
                })
                .collect();
        }
    }
    Ok(Json(dash))
}

#[derive(Debug, Deserialize)]
struct WidgetsBody {
    widgets: Vec<String>,
}

async fn put_widgets(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<WidgetsBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let widgets = state
        .members
        .save_dashboard_widgets(&who, body.widgets)
        .await
        .map_err(err)?;
    Ok(Json(serde_json::json!({ "widgets": widgets })))
}

fn empty_credits() -> jobshout_domain::DashboardCredits {
    jobshout_domain::DashboardCredits {
        balance: 0,
        expiring_soon: 0,
        plan: String::new(),
    }
}
