//! Linked Groups HTTP API — Phase 4. Gated by `GROUPS_ENABLED`.

use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_domain::DomainError;
use jobshout_groups::{
    CreateCommentRequest, CreateGroupRequest, CreatePostRequest, ReactRequest, ReportRequest,
};
use serde::Deserialize;
use serde_json::{json, Value};
use uuid::Uuid;

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/groups", get(list_groups).post(create_group))
        .route("/api/v1/groups/search", get(search_groups))
        .route("/api/v1/groups/{slug}", get(get_group))
        .route("/api/v1/groups/{slug}/join", post(join_group))
        .route(
            "/api/v1/groups/{slug}/posts",
            get(list_posts).post(create_post),
        )
        .route("/api/v1/groups/{slug}/posts/{post_id}", get(get_post))
        .route(
            "/api/v1/groups/{slug}/posts/{post_id}/comments",
            get(list_comments).post(add_comment),
        )
        .route(
            "/api/v1/groups/{slug}/posts/{post_id}/reactions",
            post(react_post),
        )
        .route("/api/v1/groups/{slug}/reports", post(report))
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

async fn viewer_member_id(state: &AppState, headers: &HeaderMap) -> Result<Option<Uuid>, ApiError> {
    if let Ok(who) = signed_in(state, headers).await {
        match state.members.ensure(&who).await {
            Ok(m) => Ok(Some(m.id)),
            Err(_) => Ok(None),
        }
    } else {
        Ok(None)
    }
}

async fn require_member_id(state: &AppState, headers: &HeaderMap) -> Result<Uuid, ApiError> {
    let who = signed_in(state, headers).await?;
    let member = state.members.ensure(&who).await.map_err(err)?;
    Ok(member.id)
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
    let viewer_member = viewer_member_id(&state, &headers).await?;
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

async fn join_group(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(slug): Path<String>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let member_id = require_member_id(&state, &headers).await?;
    let g = state
        .groups
        .get_by_slug(&slug, Some(member_id))
        .await
        .map_err(err)?;
    let role = state.groups.join(g.id, member_id).await.map_err(err)?;
    Ok(Json(json!({ "data": { "role": role, "group_id": g.id } })))
}

async fn list_posts(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(slug): Path<String>,
    Query(q): Query<ListQuery>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let viewer = viewer_member_id(&state, &headers).await?;
    let g = state.groups.get_by_slug(&slug, viewer).await.map_err(err)?;
    let rows = state
        .groups
        .list_posts(g.id, viewer, q.limit)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": rows })))
}

async fn create_post(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(slug): Path<String>,
    Json(req): Json<CreatePostRequest>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let member_id = require_member_id(&state, &headers).await?;
    let g = state
        .groups
        .get_by_slug(&slug, Some(member_id))
        .await
        .map_err(err)?;
    let post = state
        .groups
        .create_post(g.id, member_id, req)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": post })))
}

async fn get_post(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path((slug, post_id)): Path<(String, Uuid)>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let viewer = viewer_member_id(&state, &headers).await?;
    let g = state.groups.get_by_slug(&slug, viewer).await.map_err(err)?;
    let post = state
        .groups
        .get_post(g.id, post_id, viewer)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": post })))
}

async fn list_comments(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path((slug, post_id)): Path<(String, Uuid)>,
    Query(q): Query<ListQuery>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let viewer = viewer_member_id(&state, &headers).await?;
    let g = state.groups.get_by_slug(&slug, viewer).await.map_err(err)?;
    let rows = state
        .groups
        .list_comments(g.id, post_id, viewer, q.limit)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": rows })))
}

async fn add_comment(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path((slug, post_id)): Path<(String, Uuid)>,
    Json(req): Json<CreateCommentRequest>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let member_id = require_member_id(&state, &headers).await?;
    let g = state
        .groups
        .get_by_slug(&slug, Some(member_id))
        .await
        .map_err(err)?;
    let comment = state
        .groups
        .add_comment(g.id, post_id, member_id, req)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": comment })))
}

async fn react_post(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path((slug, post_id)): Path<(String, Uuid)>,
    Json(req): Json<ReactRequest>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let member_id = require_member_id(&state, &headers).await?;
    let g = state
        .groups
        .get_by_slug(&slug, Some(member_id))
        .await
        .map_err(err)?;
    let (count, liked) = state
        .groups
        .react(g.id, post_id, member_id, req)
        .await
        .map_err(err)?;
    Ok(Json(
        json!({ "data": { "reaction_count": count, "liked_by_me": liked } }),
    ))
}

async fn report(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(slug): Path<String>,
    Json(req): Json<ReportRequest>,
) -> Result<Json<Value>, ApiError> {
    require_groups(&state)?;
    let member_id = require_member_id(&state, &headers).await?;
    let g = state
        .groups
        .get_by_slug(&slug, Some(member_id))
        .await
        .map_err(err)?;
    let id = state
        .groups
        .report(g.id, member_id, req)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": { "id": id, "status": "open" } })))
}
