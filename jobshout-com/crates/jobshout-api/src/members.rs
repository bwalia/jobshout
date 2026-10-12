//! Members, follows and the following feed. Identity works as for Insights
//! (see `insights::actor`).

use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::{get, put};
use axum::{Json, Router};
use chrono::{DateTime, Utc};
use jobshout_content::Actor;
use jobshout_domain::{
    DomainError, FeedItem, FollowState, FollowedEntry, Insight, Member, MemberInput, ShowcaseApp,
};
use serde::{Deserialize, Serialize};

use crate::error::ApiError;
use crate::insights::{actor, signed_in};
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/members", get(directory))
        .route("/api/v1/members/me", get(me).patch(update_me))
        .route("/api/v1/members/{handle}", get(profile))
        .route(
            "/api/v1/members/{handle}/follow",
            put(follow).delete(unfollow),
        )
        .route("/api/v1/following", get(following))
        .route("/api/v1/following/feed", get(feed))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

/// Make sure a signed-in author has a member, so their byline links to a
/// profile. Never fails the caller's request: the member is a nicety there.
pub(crate) async fn ensure_member(state: &AppState, who: &Actor) {
    if let Err(e) = state.members.ensure(who).await {
        tracing::warn!(error = ?e, "could not create member");
    }
}

#[derive(Serialize)]
struct DataResponse<T> {
    data: T,
}

#[derive(Debug, Deserialize)]
struct DirectoryParams {
    q: Option<String>,
    #[serde(default = "default_limit")]
    limit: i64,
    #[serde(default)]
    offset: i64,
}

fn default_limit() -> i64 {
    24
}

#[derive(Serialize)]
struct DirectoryResponse {
    data: Vec<Member>,
    total: i64,
    limit: i64,
    offset: i64,
}

async fn directory(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(p): Query<DirectoryParams>,
) -> Result<Json<DirectoryResponse>, ApiError> {
    let who = actor(&state, &headers)?;
    let (data, total) = state
        .members
        .directory(p.q, who.as_ref(), p.limit, p.offset)
        .await
        .map_err(err)?;
    Ok(Json(DirectoryResponse {
        data,
        total,
        limit: p.limit.clamp(1, 50),
        offset: p.offset.max(0),
    }))
}

async fn me(State(state): State<AppState>, headers: HeaderMap) -> Result<Json<Member>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(state.members.ensure(&who).await.map_err(err)?))
}

async fn update_me(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<MemberInput>,
) -> Result<Json<Member>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(
        state.members.set_profile(&who, body).await.map_err(err)?,
    ))
}

#[derive(Serialize)]
struct ProfileResponse {
    member: Member,
    /// Whether this is the viewer's own profile.
    is_self: bool,
    /// Public, published apps, agents and teams, newest first.
    entries: Vec<ShowcaseApp>,
    /// Published Insights, newest first.
    insights: Vec<Insight>,
}

async fn profile(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(handle): Path<String>,
) -> Result<Json<ProfileResponse>, ApiError> {
    let who = actor(&state, &headers)?;
    let (member, email) = state
        .members
        .profile(&handle, who.as_ref())
        .await
        .map_err(err)?;
    let viewer_email = who.as_ref().map(|a| a.email.as_str());
    let entries = state
        .showcase
        .published_by(&email, viewer_email)
        .await
        .map_err(err)?;
    let insights = state.insights.published_by(&email).await.map_err(err)?;
    Ok(Json(ProfileResponse {
        is_self: viewer_email.is_some_and(|v| v.eq_ignore_ascii_case(&email)),
        member,
        entries,
        insights,
    }))
}

async fn set_follow(
    state: AppState,
    headers: HeaderMap,
    handle: String,
    on: bool,
) -> Result<Json<FollowState>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(
        state.members.follow(&who, &handle, on).await.map_err(err)?,
    ))
}

async fn follow(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(handle): Path<String>,
) -> Result<Json<FollowState>, ApiError> {
    set_follow(state, headers, handle, true).await
}

async fn unfollow(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(handle): Path<String>,
) -> Result<Json<FollowState>, ApiError> {
    set_follow(state, headers, handle, false).await
}

#[derive(Serialize)]
struct FollowingResponse {
    members: Vec<Member>,
    entries: Vec<FollowedEntry>,
}

async fn following(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<FollowingResponse>, ApiError> {
    let who = signed_in(&state, &headers)?;
    let (members, entries) = state.members.following(&who).await.map_err(err)?;
    Ok(Json(FollowingResponse { members, entries }))
}

#[derive(Debug, Deserialize)]
struct FeedParams {
    /// Items strictly older than this (the last item's `at`), to page back.
    before: Option<DateTime<Utc>>,
    #[serde(default = "default_feed_limit")]
    limit: i64,
}

fn default_feed_limit() -> i64 {
    30
}

async fn feed(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(p): Query<FeedParams>,
) -> Result<Json<DataResponse<Vec<FeedItem>>>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(DataResponse {
        data: state
            .members
            .feed(&who, p.before, p.limit)
            .await
            .map_err(err)?,
    }))
}
