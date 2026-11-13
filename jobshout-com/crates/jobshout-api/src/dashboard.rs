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
