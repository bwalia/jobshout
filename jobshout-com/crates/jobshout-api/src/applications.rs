use axum::extract::{Path, Query, State};
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::{CreateJobApplicationRequest, JobApplication, JobApplicationWithJob, JobId};
use serde::{Deserialize, Serialize};

use crate::error::ApiError;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route(
            "/api/v1/jobs/{id}/applications",
            get(list_for_job).post(apply),
        )
        .route("/api/v1/jobs/{id}/applications/check", get(check))
        .route("/api/v1/applications", get(list_for_email))
}

#[derive(Debug, Deserialize)]
pub struct ListQuery {
    #[serde(default = "default_limit")]
    pub limit: i64,
    #[serde(default)]
    pub offset: i64,
}

#[derive(Debug, Deserialize)]
pub struct EmailQuery {
    pub email: String,
    #[serde(default = "default_limit")]
    pub limit: i64,
}

fn default_limit() -> i64 {
    50
}

#[derive(Serialize)]
struct ApplicationListResponse {
    data: Vec<JobApplication>,
    total: i64,
    limit: i64,
    offset: i64,
}

#[derive(Serialize)]
struct MyApplicationListResponse {
    data: Vec<JobApplicationWithJob>,
    limit: i64,
}

#[derive(Serialize)]
struct AppliedResponse {
    applied: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    application: Option<JobApplication>,
}

async fn apply(
    State(state): State<AppState>,
    Path(id): Path<JobId>,
    Json(body): Json<CreateJobApplicationRequest>,
) -> Result<(axum::http::StatusCode, Json<JobApplication>), ApiError> {
    let application = state
        .applications
        .apply(id, body)
        .await
        .map_err(ApiError::from_domain)?;
    Ok((axum::http::StatusCode::CREATED, Json(application)))
}

async fn list_for_job(
    State(state): State<AppState>,
    Path(id): Path<JobId>,
    Query(q): Query<ListQuery>,
) -> Result<Json<ApplicationListResponse>, ApiError> {
    let data = state
        .applications
        .list_for_job(id, q.limit, q.offset)
        .await
        .map_err(ApiError::from_domain)?;
    let total = state
        .applications
        .count_for_job(id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(ApplicationListResponse {
        data,
        total,
        limit: q.limit.clamp(1, 100),
        offset: q.offset.max(0),
    }))
}

async fn check(
    State(state): State<AppState>,
    Path(id): Path<JobId>,
    Query(q): Query<EmailQuery>,
) -> Result<Json<AppliedResponse>, ApiError> {
    let application = state
        .applications
        .find(id, &q.email)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(AppliedResponse {
        applied: application.is_some(),
        application,
    }))
}

async fn list_for_email(
    State(state): State<AppState>,
    Query(q): Query<EmailQuery>,
) -> Result<Json<MyApplicationListResponse>, ApiError> {
    let data = state
        .applications
        .list_for_email(&q.email, q.limit)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(MyApplicationListResponse {
        data,
        limit: q.limit.clamp(1, 100),
    }))
}
