//! Linked Groups HTTP API — Phase 4. Gated by `GROUPS_ENABLED`.

use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::DomainError;
use jobshout_groups::CreateGroupRequest;
use serde::Deserialize;
use serde_json::{json, Value};

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/groups", get(list_groups).post(create_group))
        .route("/api/v1/groups/search", get(search_groups))
        .route("/api/v1/groups/{slug}", get(get_group))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_groups(state: &AppState) -> Result<(), ApiError> {
    if state.groups_enabled {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

#[derive(Debug, Deserialize)]
struct ListQuery {
    #[serde(default = "default_limit")]
    limit: i64,
}

fn default_limit() -> i64 {
    40
}

async fn list_groups(
    State(state): State<AppState>,
    Query(q): Query<ListQuery>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let rows = state.groups.list_discoverable(q.limit).await.map_err(err)?;
    Ok(Json(json!({ "data": rows })))
}

#[derive(Debug, Deserialize)]
struct SearchQuery {
    q: String,
    #[serde(default = "default_limit")]
    limit: i64,
}

async fn search_groups(
    State(state): State<AppState>,
    Query(q): Query<SearchQuery>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let rows = state.groups.search(&q.q, q.limit).await.map_err(err)?;
    Ok(Json(json!({ "data": rows })))
}

async fn get_group(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(slug): Path<String>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let viewer_member = if let Ok(who) = signed_in(&state, &headers).await {
        match state.members.ensure(&who).await {
            Ok(m) => Some(m.id),
            Err(_) => None,
        }
    } else {
        None
    };
    let g = state
        .groups
        .get_by_slug(&slug, viewer_member)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": g })))
}

async fn create_group(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(req): Json<CreateGroupRequest>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let member = state.members.ensure(&who).await.map_err(err)?;
    let g = state
        .groups
        .create(account.id, member.id, req)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": g })))
}
