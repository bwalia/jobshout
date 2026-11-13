//! Messaging HTTP API — Phase 1. Gated by `MESSAGING_ENABLED`.

use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_domain::DomainError;
use jobshout_messaging::{ConversationSummary, Message};
use serde::Deserialize;
use uuid::Uuid;

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route(
            "/api/v1/messages",
            get(list_conversations).post(open_direct),
        )
        .route(
            "/api/v1/messages/{id}",
            get(list_messages).post(send_message),
        )
        .route("/api/v1/members/{handle}/block", post(block_member))
        .route("/api/v1/members/{handle}/report", post(report_member))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_enabled(state: &AppState) -> Result<(), ApiError> {
    if state.messaging_enabled {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

#[derive(Debug, Deserialize)]
struct LimitQuery {
    #[serde(default = "default_limit")]
    limit: i64,
}

fn default_limit() -> i64 {
    30
}

async fn list_conversations(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(q): Query<LimitQuery>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let me = state.members.ensure(&who).await.map_err(err)?;
    let data: Vec<ConversationSummary> = state
        .messaging
        .list_conversations(me.id, q.limit)
        .await
        .map_err(err)?;
    Ok(Json(serde_json::json!({ "data": data })))
}

#[derive(Debug, Deserialize)]
struct OpenBody {
    handle: String,
}

async fn open_direct(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<OpenBody>,
) -> Result<Json<ConversationSummary>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let me = state.members.ensure(&who).await.map_err(err)?;
    let (peer, _) = state
        .members
        .profile(&body.handle, Some(&who))
        .await
        .map_err(err)?;
    Ok(Json(
        state
            .messaging
            .open_direct(me.id, peer.id)
            .await
            .map_err(err)?,
    ))
}

async fn list_messages(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<Uuid>,
    Query(q): Query<LimitQuery>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let me = state.members.ensure(&who).await.map_err(err)?;
    let data: Vec<Message> = state
        .messaging
        .list_messages(me.id, id, q.limit)
        .await
        .map_err(err)?;
    Ok(Json(serde_json::json!({ "data": data })))
}

#[derive(Debug, Deserialize)]
struct SendBody {
    body: String,
}

async fn send_message(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<Uuid>,
    Json(body): Json<SendBody>,
) -> Result<Json<Message>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let me = state.members.ensure(&who).await.map_err(err)?;
    Ok(Json(
        state
            .messaging
            .send(me.id, id, &body.body)
            .await
            .map_err(err)?,
    ))
}

async fn block_member(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(handle): Path<String>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let me = state.members.ensure(&who).await.map_err(err)?;
    let (peer, _) = state
        .members
        .profile(&handle, Some(&who))
        .await
        .map_err(err)?;
    state.messaging.block(me.id, peer.id).await.map_err(err)?;
    Ok(Json(serde_json::json!({ "blocked": true })))
}

#[derive(Debug, Deserialize)]
struct ReportBody {
    reason: String,
    #[serde(default)]
    detail: String,
}

async fn report_member(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(handle): Path<String>,
    Json(body): Json<ReportBody>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_enabled(&state)?;
    let who = signed_in(&state, &headers).await?;
    let me = state.members.ensure(&who).await.map_err(err)?;
    let (peer, _) = state
        .members
        .profile(&handle, Some(&who))
        .await
        .map_err(err)?;
    let id = state
        .messaging
        .report(me.id, peer.id, &body.reason, &body.detail)
        .await
        .map_err(err)?;
    Ok(Json(serde_json::json!({ "id": id })))
}
