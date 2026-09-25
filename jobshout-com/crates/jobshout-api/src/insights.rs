//! Insights hub routes.
//!
//! Identity: the web tier forwards the signed-in user as
//! `x-jobshout-user-email` / `x-jobshout-user-name`, and platform agents (the
//! Article Writer) identify as `x-jobshout-agent`; both are signed with the
//! shared `x-jobshout-internal-token`. The gateway also exposes `/api/` publicly, so
//! a request without the matching token is treated as a forgery, and the
//! gateway strips these headers from public traffic as a second line.

use axum::extract::{Path, Query, State};
use axum::http::{header, HeaderMap, StatusCode};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_content::feed::{self, FeedSite};
use jobshout_content::{Actor, ListQuery, Moderation};
use jobshout_domain::{
    DomainError, Insight, InsightInput, InsightKind, InsightTopic, NewsletterDigest,
};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::error::ApiError;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/insights", get(list).post(create))
        .route("/api/v1/insights/topics", get(topics))
        .route("/api/v1/insights/mine", get(mine))
        .route("/api/v1/insights/whoami", get(whoami))
        .route("/api/v1/insights/review-queue", get(review_queue))
        .route("/api/v1/insights/digests", get(digests))
        .route("/api/v1/insights/preview", post(preview))
        .route("/api/v1/insights/feed/rss", get(rss))
        .route("/api/v1/insights/feed/podcast", get(podcast))
        .route(
            "/api/v1/insights/{key}",
            get(get_one).patch(update).delete(remove),
        )
        .route("/api/v1/insights/{key}/related", get(related))
        .route("/api/v1/insights/{key}/moderate", post(moderate))
        .route("/api/v1/newsletter/subscribe", post(subscribe))
        .route("/api/v1/newsletter/confirm", post(confirm))
        .route("/api/v1/newsletter/unsubscribe", post(unsubscribe))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn bad_request(msg: impl Into<String>) -> ApiError {
    err(DomainError::Validation(msg.into()))
}

fn same(a: &[u8], b: &[u8]) -> bool {
    a.len() == b.len() && a.iter().zip(b).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}

/// The signed-in user, if the web tier forwarded one.
fn actor(state: &AppState, headers: &HeaderMap) -> Result<Option<Actor>, ApiError> {
    let text = |name: &str| {
        headers
            .get(name)
            .and_then(|v| v.to_str().ok())
            .map(str::trim)
            .unwrap_or_default()
            .to_string()
    };
    let email = text("x-jobshout-user-email");
    let agent = text("x-jobshout-agent");
    if email.is_empty() && agent.is_empty() {
        return Ok(None);
    }
    if let Some(expected) = &state.internal_token {
        if !same(
            text("x-jobshout-internal-token").as_bytes(),
            expected.as_bytes(),
        ) {
            return Err(err(DomainError::Unauthorized(
                "untrusted identity headers".into(),
            )));
        }
    }
    // An agent header wins over a user header: a platform agent acts as
    // itself, never on behalf of whoever launched it.
    if !agent.is_empty() {
        return state.insights.agent_actor(&agent).map(Some).map_err(err);
    }
    Ok(Some(
        state.insights.actor(&email, &text("x-jobshout-user-name")),
    ))
}

fn signed_in(state: &AppState, headers: &HeaderMap) -> Result<Actor, ApiError> {
    actor(state, headers)?
        .ok_or_else(|| err(DomainError::Unauthorized("sign in to do that".into())))
}

fn parse_id(key: &str) -> Result<Uuid, ApiError> {
    Uuid::parse_str(key).map_err(|_| bad_request("expected an insight id"))
}

fn parse_kind(kind: Option<&str>) -> Result<Option<InsightKind>, ApiError> {
    match kind.map(str::trim).filter(|k| !k.is_empty() && *k != "all") {
        None => Ok(None),
        Some(k) => InsightKind::parse(k)
            .map(Some)
            .ok_or_else(|| bad_request(format!("invalid kind: {k}"))),
    }
}

/* --- Reading ------------------------------------------------------------- */

#[derive(Debug, Deserialize)]
struct ListParams {
    kind: Option<String>,
    topic: Option<String>,
    q: Option<String>,
    featured: Option<bool>,
    #[serde(default = "default_limit")]
    limit: i64,
    #[serde(default)]
    offset: i64,
}

fn default_limit() -> i64 {
    12
}

#[derive(Serialize)]
struct ListResponse {
    data: Vec<Insight>,
    total: i64,
    limit: i64,
    offset: i64,
}

#[derive(Serialize)]
struct DataResponse<T> {
    data: T,
}

async fn list(
    State(state): State<AppState>,
    Query(p): Query<ListParams>,
) -> Result<Json<ListResponse>, ApiError> {
    let q = ListQuery {
        kind: parse_kind(p.kind.as_deref())?,
        topic: p.topic.filter(|t| !t.trim().is_empty()),
        q: p.q,
        featured: p.featured,
        limit: p.limit,
        offset: p.offset,
        ..Default::default()
    };
    let (data, total) = state.insights.list_published(q).await.map_err(err)?;
    Ok(Json(ListResponse {
        data,
        total,
        limit: p.limit.clamp(1, 50),
        offset: p.offset.max(0),
    }))
}

async fn topics(
    State(state): State<AppState>,
) -> Result<Json<DataResponse<Vec<InsightTopic>>>, ApiError> {
    Ok(Json(DataResponse {
        data: state.insights.topics().await.map_err(err)?,
    }))
}

async fn get_one(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
) -> Result<Json<Insight>, ApiError> {
    let who = actor(&state, &headers)?;
    Ok(Json(
        state
            .insights
            .get_by_slug(&key, who.as_ref())
            .await
            .map_err(err)?,
    ))
}

#[derive(Debug, Deserialize)]
struct RelatedParams {
    #[serde(default = "default_related")]
    limit: i64,
}

fn default_related() -> i64 {
    3
}

async fn related(
    State(state): State<AppState>,
    Path(key): Path<String>,
    Query(p): Query<RelatedParams>,
) -> Result<Json<DataResponse<Vec<Insight>>>, ApiError> {
    Ok(Json(DataResponse {
        data: state
            .insights
            .related(&key, p.limit.clamp(1, 12))
            .await
            .map_err(err)?,
    }))
}

async fn mine(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<DataResponse<Vec<Insight>>>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(DataResponse {
        data: state.insights.list_mine(&who).await.map_err(err)?,
    }))
}

#[derive(Serialize)]
struct WhoAmI {
    email: String,
    is_staff: bool,
}

async fn whoami(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<WhoAmI>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(WhoAmI {
        email: who.email,
        is_staff: who.is_staff,
    }))
}

async fn review_queue(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<DataResponse<Vec<Insight>>>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(DataResponse {
        data: state.insights.review_queue(&who).await.map_err(err)?,
    }))
}

#[derive(Debug, Deserialize)]
struct DigestParams {
    #[serde(default = "default_weeks")]
    weeks: i64,
}

fn default_weeks() -> i64 {
    12
}

async fn digests(
    State(state): State<AppState>,
    Query(p): Query<DigestParams>,
) -> Result<Json<DataResponse<Vec<NewsletterDigest>>>, ApiError> {
    Ok(Json(DataResponse {
        data: state.insights.digests(p.weeks).await.map_err(err)?,
    }))
}

#[derive(Debug, Deserialize)]
struct PreviewBody {
    #[serde(default)]
    body_md: String,
}

#[derive(Serialize)]
struct PreviewResponse {
    html: String,
}

async fn preview(
    State(state): State<AppState>,
    Json(body): Json<PreviewBody>,
) -> Result<Json<PreviewResponse>, ApiError> {
    if body.body_md.len() > 200_000 {
        return Err(bad_request("that is too long to preview"));
    }
    Ok(Json(PreviewResponse {
        html: state.insights.preview(&body.body_md),
    }))
}

/* --- Feeds --------------------------------------------------------------- */

fn xml(body: String) -> Response {
    (
        [
            (header::CONTENT_TYPE, "application/rss+xml; charset=utf-8"),
            (header::CACHE_CONTROL, "public, max-age=300"),
        ],
        body,
    )
        .into_response()
}

async fn rss(State(state): State<AppState>) -> Result<Response, ApiError> {
    let items = state.insights.feed_items(None).await.map_err(err)?;
    Ok(xml(feed::rss(
        &FeedSite {
            base: &state.site_url,
        },
        &items,
    )))
}

async fn podcast(State(state): State<AppState>) -> Result<Response, ApiError> {
    let items = state
        .insights
        .feed_items(Some(InsightKind::Podcast))
        .await
        .map_err(err)?;
    let cover = format!("{}/insights/podcast-cover.png", state.site_url);
    Ok(xml(feed::podcast(
        &FeedSite {
            base: &state.site_url,
        },
        &cover,
        &items,
    )))
}

/* --- Writing ------------------------------------------------------------- */

async fn create(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<InsightInput>,
) -> Result<(StatusCode, Json<Insight>), ApiError> {
    let who = signed_in(&state, &headers)?;
    let item = state.insights.create(&who, body).await.map_err(err)?;
    Ok((StatusCode::CREATED, Json(item)))
}

async fn update(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
    Json(body): Json<InsightInput>,
) -> Result<Json<Insight>, ApiError> {
    let who = signed_in(&state, &headers)?;
    let id = parse_id(&key)?;
    Ok(Json(
        state.insights.update(&who, id, body).await.map_err(err)?,
    ))
}

async fn remove(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
) -> Result<StatusCode, ApiError> {
    let who = signed_in(&state, &headers)?;
    state
        .insights
        .delete(&who, parse_id(&key)?)
        .await
        .map_err(err)?;
    Ok(StatusCode::NO_CONTENT)
}

#[derive(Debug, Deserialize)]
struct ModerateBody {
    /// approve | reject | feature | unfeature | archive
    action: String,
    #[serde(default)]
    note: String,
}

async fn moderate(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
    Json(body): Json<ModerateBody>,
) -> Result<Json<Insight>, ApiError> {
    let who = signed_in(&state, &headers)?;
    let action = match body.action.as_str() {
        "approve" => Moderation::Approve,
        "reject" => Moderation::Reject { note: body.note },
        "feature" => Moderation::Feature(true),
        "unfeature" => Moderation::Feature(false),
        "archive" => Moderation::Archive,
        other => return Err(bad_request(format!("unknown action: {other}"))),
    };
    Ok(Json(
        state
            .insights
            .moderate(&who, parse_id(&key)?, action)
            .await
            .map_err(err)?,
    ))
}

/* --- Newsletter ---------------------------------------------------------- */

#[derive(Debug, Deserialize)]
struct SubscribeBody {
    email: String,
}

#[derive(Debug, Deserialize)]
struct TokenBody {
    token: String,
}

#[derive(Serialize)]
struct SubscriptionResponse {
    status: &'static str,
}

async fn subscribe(
    State(state): State<AppState>,
    Json(body): Json<SubscribeBody>,
) -> Result<(StatusCode, Json<SubscriptionResponse>), ApiError> {
    match state.insights.subscribe(&body.email).await.map_err(err)? {
        Some(token) => {
            // No mailer is wired up yet: the confirmation link goes to the log
            // so the flow can be exercised end to end. Swap for an email send.
            tracing::info!(
                confirm_url = %format!("{}/newsletter/confirm?token={token}", state.site_url),
                "newsletter confirmation pending (no mailer configured)"
            );
            Ok((
                StatusCode::ACCEPTED,
                Json(SubscriptionResponse { status: "pending" }),
            ))
        }
        // Same answer either way, so the form cannot be used to probe the list.
        None => Ok((
            StatusCode::ACCEPTED,
            Json(SubscriptionResponse { status: "pending" }),
        )),
    }
}

async fn confirm(
    State(state): State<AppState>,
    Json(body): Json<TokenBody>,
) -> Result<Json<SubscriptionResponse>, ApiError> {
    state
        .insights
        .confirm(body.token.trim())
        .await
        .map_err(err)?;
    Ok(Json(SubscriptionResponse {
        status: "confirmed",
    }))
}

async fn unsubscribe(
    State(state): State<AppState>,
    Json(body): Json<TokenBody>,
) -> Result<Json<SubscriptionResponse>, ApiError> {
    state
        .insights
        .unsubscribe(body.token.trim())
        .await
        .map_err(err)?;
    Ok(Json(SubscriptionResponse {
        status: "unsubscribed",
    }))
}
