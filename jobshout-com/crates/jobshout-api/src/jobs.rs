use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::{CreateJobRequest, Job, JobId, JobStatus};
use jobshout_policy::{require, Action, Resource};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::error::ApiError;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/jobs", get(list_jobs).post(create_job))
        .route("/api/v1/jobs/{id}", get(get_job).patch(update_job))
        .route("/api/v1/jobs/{id}/close", axum::routing::post(close_job))
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

async fn require_account(
    state: &AppState,
    headers: &HeaderMap,
) -> Result<jobshout_auth::Account, ApiError> {
    state
        .accounts
        .require(headers, state.internal_token.as_deref())
        .await
        .map_err(ApiError::from_domain)
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
    headers: HeaderMap,
    Json(body): Json<CreateJobRequest>,
) -> Result<(axum::http::StatusCode, Json<Job>), ApiError> {
    let account = require_account(&state, &headers).await?;
    require(&account, Action::PostJob, &Resource::Site).map_err(ApiError::from_domain)?;
    let job = state
        .jobs
        .create(body, &account.primary_email)
        .await
        .map_err(ApiError::from_domain)?;
    let _ = state
        .events
        .emit(
            "job.posted",
            "job",
            &job.id.to_string(),
            Some(account.id),
            serde_json::json!({ "title": job.title, "status": job.status }),
        )
        .await;
    Ok((axum::http::StatusCode::CREATED, Json(job)))
}

async fn update_job(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
    Json(body): Json<CreateJobRequest>,
) -> Result<Json<Job>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let poster = state
        .jobs
        .poster_email(id)
        .await
        .map_err(ApiError::from_domain)?;
    require(
        &account,
        Action::UpdateJob,
        &Resource::Job {
            poster_email: poster,
            poster_account_id: None,
        },
    )
    .map_err(ApiError::from_domain)?;
    let job = state
        .jobs
        .update(id, body)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(job))
}

async fn close_job(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
) -> Result<Json<Job>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let poster = state
        .jobs
        .poster_email(id)
        .await
        .map_err(ApiError::from_domain)?;
    require(
        &account,
        Action::CloseJob,
        &Resource::Job {
            poster_email: poster,
            poster_account_id: None,
        },
    )
    .map_err(ApiError::from_domain)?;
    let job = state.jobs.close(id).await.map_err(ApiError::from_domain)?;
    let _ = state
        .events
        .emit(
            "job.closed",
            "job",
            &job.id.to_string(),
            Some(account.id),
            serde_json::json!({ "title": job.title }),
        )
        .await;
    Ok(Json(job))
}
