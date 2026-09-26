//! AI Showcase routes. Identity works exactly as for Insights (see
//! `insights::actor`): signed headers from the web tier, or an agent header.

use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, StatusCode};
use axum::routing::{get, post, put};
use axum::{Json, Router};
use jobshout_domain::{
    DomainError, ShowcaseApp, ShowcaseAppInput, ShowcaseAppType, ShowcaseBuildMethod,
    ShowcaseMaturity, ShowcasePricing, ShowcaseTag,
};
use jobshout_showcase::{ListQuery, Moderation, Sort};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::error::ApiError;
use crate::insights::{actor, signed_in};
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/showcase/apps", get(list).post(create))
        .route("/api/v1/showcase/apps/mine", get(mine))
        .route("/api/v1/showcase/tags", get(tags))
        .route("/api/v1/showcase/review-queue", get(review_queue))
        .route(
            "/api/v1/showcase/apps/{key}",
            get(get_one).patch(update).delete(remove),
        )
        .route("/api/v1/showcase/apps/{key}/related", get(related))
        .route("/api/v1/showcase/apps/{key}/moderate", post(moderate))
        .route("/api/v1/showcase/apps/{key}/star", put(star).delete(unstar))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn bad_request(msg: impl Into<String>) -> ApiError {
    err(DomainError::Validation(msg.into()))
}

fn parse_id(key: &str) -> Result<Uuid, ApiError> {
    Uuid::parse_str(key).map_err(|_| bad_request("expected an app id"))
}

/// An optional enum filter: empty or "all" means no filter.
fn parse_opt<T>(
    what: &str,
    raw: Option<&str>,
    parse: fn(&str) -> Option<T>,
) -> Result<Option<T>, ApiError> {
    match raw.map(str::trim).filter(|s| !s.is_empty() && *s != "all") {
        None => Ok(None),
        Some(s) => parse(s)
            .map(Some)
            .ok_or_else(|| bad_request(format!("invalid {what}: {s}"))),
    }
}

#[derive(Debug, Deserialize)]
struct ListParams {
    q: Option<String>,
    #[serde(rename = "type")]
    app_type: Option<String>,
    maturity: Option<String>,
    build: Option<String>,
    pricing: Option<String>,
    tech: Option<String>,
    /// featured | production_ready | built_by_agents | open_source
    collection: Option<String>,
    featured: Option<bool>,
    /// new (default) | stars | updated
    sort: Option<String>,
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
    data: Vec<ShowcaseApp>,
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
    headers: HeaderMap,
    Query(p): Query<ListParams>,
) -> Result<Json<ListResponse>, ApiError> {
    let who = actor(&state, &headers)?;
    let mut q = ListQuery {
        q: p.q,
        app_type: parse_opt("type", p.app_type.as_deref(), ShowcaseAppType::parse)?,
        maturities: parse_opt("maturity", p.maturity.as_deref(), ShowcaseMaturity::parse)?
            .into_iter()
            .collect(),
        build_methods: parse_opt("build", p.build.as_deref(), ShowcaseBuildMethod::parse)?
            .into_iter()
            .collect(),
        pricing: parse_opt("pricing", p.pricing.as_deref(), ShowcasePricing::parse)?,
        technology: p.tech,
        featured: p.featured,
        viewer_email: who.map(|a| a.email),
        sort: match p.sort.as_deref().unwrap_or("new") {
            "new" | "" => Sort::Newest,
            "stars" => Sort::Stars,
            "updated" => Sort::Updated,
            other => return Err(bad_request(format!("invalid sort: {other}"))),
        },
        limit: p.limit,
        offset: p.offset,
        ..Default::default()
    };
    // Collections are named slices the landing page links to; they narrow
    // whatever explicit filters were also given.
    match p.collection.as_deref().map(str::trim).unwrap_or("") {
        "" | "all" => {}
        "featured" => q.featured = Some(true),
        "production_ready" if q.maturities.is_empty() => {
            q.maturities = vec![
                ShowcaseMaturity::ProductionReady,
                ShowcaseMaturity::EnterpriseReady,
            ]
        }
        "built_by_agents" if q.build_methods.is_empty() => {
            q.build_methods = vec![
                ShowcaseBuildMethod::AgentBuilt,
                ShowcaseBuildMethod::AgentAutonomous,
            ]
        }
        "open_source" => q.pricing = Some(ShowcasePricing::OpenSource),
        "production_ready" | "built_by_agents" => {}
        other => return Err(bad_request(format!("invalid collection: {other}"))),
    }
    let (data, total) = state.showcase.list_published(q).await.map_err(err)?;
    Ok(Json(ListResponse {
        data,
        total,
        limit: p.limit.clamp(1, 50),
        offset: p.offset.max(0),
    }))
}

#[derive(Debug, Deserialize)]
struct LimitParams {
    limit: Option<i64>,
}

async fn tags(
    State(state): State<AppState>,
    Query(p): Query<LimitParams>,
) -> Result<Json<DataResponse<Vec<ShowcaseTag>>>, ApiError> {
    Ok(Json(DataResponse {
        data: state
            .showcase
            .tags(p.limit.unwrap_or(24))
            .await
            .map_err(err)?,
    }))
}

async fn get_one(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
) -> Result<Json<ShowcaseApp>, ApiError> {
    let who = actor(&state, &headers)?;
    Ok(Json(
        state
            .showcase
            .get_by_slug(&key, who.as_ref())
            .await
            .map_err(err)?,
    ))
}

async fn related(
    State(state): State<AppState>,
    Path(key): Path<String>,
    Query(p): Query<LimitParams>,
) -> Result<Json<DataResponse<Vec<ShowcaseApp>>>, ApiError> {
    Ok(Json(DataResponse {
        data: state
            .showcase
            .related(&key, p.limit.unwrap_or(3).clamp(1, 12))
            .await
            .map_err(err)?,
    }))
}

async fn mine(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<DataResponse<Vec<ShowcaseApp>>>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(DataResponse {
        data: state.showcase.list_mine(&who).await.map_err(err)?,
    }))
}

async fn review_queue(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<DataResponse<Vec<ShowcaseApp>>>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(DataResponse {
        data: state.showcase.review_queue(&who).await.map_err(err)?,
    }))
}

async fn create(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<ShowcaseAppInput>,
) -> Result<(StatusCode, Json<ShowcaseApp>), ApiError> {
    let who = signed_in(&state, &headers)?;
    let app = state.showcase.create(&who, body).await.map_err(err)?;
    Ok((StatusCode::CREATED, Json(app)))
}

async fn update(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
    Json(body): Json<ShowcaseAppInput>,
) -> Result<Json<ShowcaseApp>, ApiError> {
    let who = signed_in(&state, &headers)?;
    Ok(Json(
        state
            .showcase
            .update(&who, parse_id(&key)?, body)
            .await
            .map_err(err)?,
    ))
}

async fn remove(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
) -> Result<StatusCode, ApiError> {
    let who = signed_in(&state, &headers)?;
    state
        .showcase
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
) -> Result<Json<ShowcaseApp>, ApiError> {
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
            .showcase
            .moderate(&who, parse_id(&key)?, action)
            .await
            .map_err(err)?,
    ))
}

#[derive(Serialize)]
struct StarResponse {
    starred: bool,
    star_count: i32,
}

async fn set_star(
    state: AppState,
    headers: HeaderMap,
    slug: String,
    on: bool,
) -> Result<Json<StarResponse>, ApiError> {
    let who = signed_in(&state, &headers)?;
    let star_count = state.showcase.star(&who, &slug, on).await.map_err(err)?;
    Ok(Json(StarResponse {
        starred: on,
        star_count,
    }))
}

async fn star(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
) -> Result<Json<StarResponse>, ApiError> {
    set_star(state, headers, key, true).await
}

async fn unstar(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(key): Path<String>,
) -> Result<Json<StarResponse>, ApiError> {
    set_star(state, headers, key, false).await
}
