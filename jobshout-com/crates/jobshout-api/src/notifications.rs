//! In-app notifications: list, unread count, mark read, and per-kind
//! preferences. Always on; rows are only ever read by their own account.

use axum::extract::{Query, State};
use axum::http::HeaderMap;
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_notifications::{Notification, Preference};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::error::ApiError;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/notifications", get(list))
        .route("/api/v1/notifications/read", post(mark_read))
        .route(
            "/api/v1/notifications/preferences",
            get(preferences).put(set_preference),
        )
}

async fn account(
    state: &AppState,
    headers: &HeaderMap,
) -> Result<jobshout_auth::Account, ApiError> {
    state
        .accounts
        .require(headers, state.internal_token.as_deref())
        .await
        .map_err(ApiError::from_domain)
}

#[derive(Debug, Deserialize)]
struct ListQuery {
    #[serde(default = "default_limit")]
    limit: i64,
    #[serde(default)]
    unread: bool,
}

fn default_limit() -> i64 {
    30
}

#[derive(Serialize)]
struct ListResponse {
    data: Vec<Notification>,
    unread: i64,
}

async fn list(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(q): Query<ListQuery>,
) -> Result<Json<ListResponse>, ApiError> {
    let me = account(&state, &headers).await?;
    let data = state
        .notifications
        .list(me.id, q.limit, q.unread)
        .await
        .map_err(ApiError::from_domain)?;
    let unread = state
        .notifications
        .unread_count(me.id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(ListResponse { data, unread }))
}

#[derive(Debug, Deserialize)]
struct ReadBody {
    /// Empty (or omitted) marks everything read.
    #[serde(default)]
    ids: Vec<Uuid>,
}

async fn mark_read(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<ReadBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let me = account(&state, &headers).await?;
    let marked = state
        .notifications
        .mark_read(me.id, &body.ids)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(serde_json::json!({ "marked": marked })))
}

async fn preferences(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<Vec<Preference>>, ApiError> {
    let me = account(&state, &headers).await?;
    Ok(Json(
        state
            .notifications
            .preferences(me.id)
            .await
            .map_err(ApiError::from_domain)?,
    ))
}

#[derive(Debug, Deserialize)]
struct PreferenceBody {
    kind: String,
    in_app: bool,
}

async fn set_preference(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<PreferenceBody>,
) -> Result<Json<Vec<Preference>>, ApiError> {
    let me = account(&state, &headers).await?;
    state
        .notifications
        .set_preference(me.id, &body.kind, body.in_app)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(
        state
            .notifications
            .preferences(me.id)
            .await
            .map_err(ApiError::from_domain)?,
    ))
}
