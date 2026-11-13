//! Teams & agencies + analytics — Phase 6. Gated by `TEAMS_ENABLED`.

use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::get;
use axum::{Json, Router};
use jobshout_analytics::CreateTeamRequest;
use jobshout_domain::DomainError;
use serde::Deserialize;
use serde_json::{json, Value};

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/teams", get(list_teams).post(create_team))
        .route(
            "/api/v1/analytics/{subject_type}/{subject_id}",
            get(metrics),
        )
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_teams(state: &AppState) -> Result<(), ApiError> {
    if state.teams_enabled {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

async fn list_teams(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<Value>, ApiError> {
    require_teams(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let rows = state
        .analytics
        .list_teams_for_owner(account.id)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": rows })))
}

async fn create_team(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(req): Json<CreateTeamRequest>,
) -> Result<Json<Value>, ApiError> {
    require_teams(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let team = state
        .analytics
        .create_team(account.id, req)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": team })))
}

#[derive(Debug, Deserialize)]
struct MetricsQuery {
    #[serde(default = "default_limit")]
    limit: i64,
}

fn default_limit() -> i64 {
    30
}

async fn metrics(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path((subject_type, subject_id)): Path<(String, String)>,
    Query(q): Query<MetricsQuery>,
) -> Result<Json<Value>, ApiError> {
    require_teams(&state)?;
    let _ = signed_in(&state, &headers).await?;
    let rows = state
        .analytics
        .metrics_for(&subject_type, &subject_id, q.limit)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": rows })))
}
