//! Projects marketplace HTTP API — Phase 2a. Gated by `PROJECTS_LIVE`.

use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_domain::{
    Bid, BidStatus, CreateBidRequest, CreateProjectRequest, DomainError, Project, ProjectType,
    UpsertCandidateProfileRequest,
};
use serde::Deserialize;
use uuid::Uuid;

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/projects", get(list_projects).post(create_project))
        .route("/api/v1/projects/categories", get(list_categories))
        .route("/api/v1/projects/{id}", get(get_project))
        .route(
            "/api/v1/projects/{id}/bids",
            get(list_bids).post(submit_bid),
        )
        .route("/api/v1/bids/mine", get(my_bids))
        .route("/api/v1/bids/{id}/status", post(update_bid_status))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_live(state: &AppState) -> Result<(), ApiError> {
    if state.projects_live {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

async fn ensure_candidate(
    state: &AppState,
    email: &str,
    name: &str,
) -> Result<jobshout_domain::CandidateProfile, ApiError> {
    match state.candidates.get_by_email(email).await {
        Ok(p) => Ok(p),
        Err(DomainError::NotFound) => state
            .candidates
            .upsert(UpsertCandidateProfileRequest {
                email: email.into(),
                display_name: if name.trim().is_empty() {
                    email.split('@').next().unwrap_or("member").into()
                } else {
                    name.into()
                },
                headline: String::new(),
                summary: "Freelancer profile created when bidding.".into(),
                skills: vec!["general".into()],
                years_experience: None,
                preferred_roles: Vec::new(),
                preferred_locations: Vec::new(),
                preferred_employment_types: Vec::new(),
                open_to_remote: true,
                salary_expectation: jobshout_domain::Compensation {
                    currency: None,
                    min_amount: None,
                    max_amount: None,
                    period: None,
                },
                cv_text: String::new(),
                matching_notes: String::new(),
            })
            .await
            .map_err(err),
        Err(e) => Err(err(e)),
    }
}

#[derive(Debug, Deserialize)]
struct ListQuery {
    #[serde(default = "default_limit")]
    limit: i64,
    #[serde(default)]
    offset: i64,
    project_type: Option<ProjectType>,
}

fn default_limit() -> i64 {
    20
}

async fn list_projects(
    State(state): State<AppState>,
    Query(q): Query<ListQuery>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_live(&state)?;
    let projects = state
        .freelance
        .list_projects(q.limit, q.offset, q.project_type, None)
        .await
        .map_err(err)?;
    Ok(Json(serde_json::json!({ "data": projects })))
}

async fn create_project(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(req): Json<CreateProjectRequest>,
) -> Result<Json<Project>, ApiError> {
    require_live(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    Ok(Json(
        state
            .freelance
            .create_project(req, account.id)
            .await
            .map_err(err)?,
    ))
}

async fn get_project(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
) -> Result<Json<Project>, ApiError> {
    require_live(&state)?;
    Ok(Json(state.freelance.get_project(id).await.map_err(err)?))
}

#[derive(Debug, Deserialize)]
struct BidsQuery {
    #[serde(default = "default_limit")]
    limit: i64,
    #[serde(default)]
    offset: i64,
    status: Option<BidStatus>,
}

async fn list_bids(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<Uuid>,
    Query(q): Query<BidsQuery>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_live(&state)?;
    let who = signed_in(&state, &headers).await?;
    let project = state.freelance.get_project(id).await.map_err(err)?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let is_owner = project.owner_account_id == Some(account.id);
    if !is_owner && !account.is_staff() {
        return Err(ApiError::from_domain(DomainError::Forbidden(
            "only the project owner can list bids".into(),
        )));
    }
    let bids = state
        .freelance
        .list_bids(id, q.limit, q.offset, q.status)
        .await
        .map_err(err)?;
    Ok(Json(serde_json::json!({ "data": bids })))
}

async fn submit_bid(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<Uuid>,
    Json(req): Json<CreateBidRequest>,
) -> Result<Json<Bid>, ApiError> {
    require_live(&state)?;
    let who = signed_in(&state, &headers).await?;
    let profile = ensure_candidate(&state, &who.email, &who.name).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let project = state.freelance.get_project(id).await.map_err(err)?;
    let mut bid = state
        .freelance
        .submit_bid(id, req, profile.id)
        .await
        .map_err(err)?;
    if state.credits_enabled {
        let cost = jobshout_billing::bid_cost_for_budget(
            jobshout_freelance::FreelanceService::project_budget_for_cost(&project),
        );
        let spent = state
            .billing
            .spend_for_bid(account.id, bid.id, cost)
            .await
            .map_err(err)?;
        if spent > 0 {
            bid = state
                .freelance
                .set_bid_credits_spent(bid.id, spent)
                .await
                .map_err(err)?;
        }
    }
    Ok(Json(bid))
}

async fn my_bids(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(q): Query<BidsQuery>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_live(&state)?;
    let who = signed_in(&state, &headers).await?;
    let profile = ensure_candidate(&state, &who.email, &who.name).await?;
    let bids = state
        .freelance
        .list_my_bids(profile.id, q.limit, q.offset, q.status)
        .await
        .map_err(err)?;
    Ok(Json(serde_json::json!({ "data": bids })))
}

#[derive(Debug, Deserialize)]
struct StatusBody {
    status: BidStatus,
}

async fn update_bid_status(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<Uuid>,
    Json(body): Json<StatusBody>,
) -> Result<Json<Bid>, ApiError> {
    require_live(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    Ok(Json(
        state
            .freelance
            .update_bid_status(id, body.status, account.id)
            .await
            .map_err(err)?,
    ))
}

async fn list_categories(
    State(state): State<AppState>,
) -> Result<Json<serde_json::Value>, ApiError> {
    require_live(&state)?;
    let categories = state.freelance.list_categories().await.map_err(err)?;
    Ok(Json(serde_json::json!({ "data": categories })))
}
