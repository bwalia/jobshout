use axum::extract::{Path, Query, State};
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::{CreateJobRequest, Job, JobId, JobStatus};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::error::ApiError;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/jobs", get(list_jobs).post(create_job))
        .route("/api/v1/jobs/{id}", get(get_job))
}

#[derive(Debug, Deserialize)]
pub struct ListJobsQuery {
    #[serde(default = "default_limit")]
    pub limit: i64,
    #[serde(default)]
    pub offset: i64,
    /// draft | published | closed | archived — default published for public board
    pub status: Option<String>,
}

fn default_limit() -> i64 {
    20
}

#[derive(Serialize)]
struct JobListResponse {
    data: Vec<Job>,
    limit: i64,
    offset: i64,
}

async fn list_jobs(
    State(state): State<AppState>,
    Query(q): Query<ListJobsQuery>,
) -> Result<Json<JobListResponse>, ApiError> {
    let status = match q.status.as_deref() {
        None => Some(JobStatus::Published),
        Some("all") => None,
        Some(s) => Some(JobStatus::parse(s).ok_or_else(|| ApiError {
            status: axum::http::StatusCode::BAD_REQUEST,
            code: "VALIDATION_ERROR",
            message: format!("invalid status: {s}"),
            request_id: Uuid::new_v4().to_string(),
        })?),
    };
    let data = state
        .jobs
        .list(q.limit, q.offset, status)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(JobListResponse {
        data,
        limit: q.limit.clamp(1, 100),
        offset: q.offset.max(0),
    }))
}

async fn get_job(
    State(state): State<AppState>,
    Path(id): Path<JobId>,
) -> Result<Json<Job>, ApiError> {
    let job = state.jobs.get(id).await.map_err(ApiError::from_domain)?;
    Ok(Json(job))
}

async fn create_job(
    State(state): State<AppState>,
    Json(body): Json<CreateJobRequest>,
) -> Result<(axum::http::StatusCode, Json<Job>), ApiError> {
    let job = state
        .jobs
        .create(body)
        .await
        .map_err(ApiError::from_domain)?;
    Ok((axum::http::StatusCode::CREATED, Json(job)))
}
