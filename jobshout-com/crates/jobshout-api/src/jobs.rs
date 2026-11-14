use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::{CreateJobRequest, DomainError, Job, JobId, JobStatus};
use jobshout_policy::{require, Action, Resource};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::error::ApiError;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/jobs", get(list_jobs).post(create_job))
        .route("/api/v1/jobs/mine", get(list_my_jobs))
        .route(
            "/api/v1/jobs/{id}",
            get(get_job).patch(update_job).delete(delete_job),
        )
        .route("/api/v1/jobs/{id}/close", axum::routing::post(close_job))
        .route(
            "/api/v1/jobs/{id}/publish",
            axum::routing::post(publish_job),
        )
        .route("/api/v1/jobs/saved", get(list_saved))
        .route(
            "/api/v1/jobs/{id}/save",
            get(saved_state).put(save_job).delete(unsave_job),
        )
        .route("/api/v1/job-alerts", get(list_alerts).post(create_alert))
        .route(
            "/api/v1/job-alerts/{alert_id}",
            axum::routing::put(update_alert).delete(delete_alert),
        )
}

#[derive(Debug, Deserialize)]
pub struct ListJobsQuery {
    #[serde(default = "default_limit")]
    pub limit: i64,
    #[serde(default)]
    pub offset: i64,
    /// draft | published | closed | archived — default published for public board
    pub status: Option<String>,
    /// Keyword over title, summary, description and requirements.
    pub q: Option<String>,
    /// City / region / country; "remote" or "anywhere" means remote roles.
    #[serde(rename = "where")]
    pub location: Option<String>,
    /// Comma-separated employment types.
    #[serde(rename = "type")]
    pub types: Option<String>,
    /// `1` / `true` for remote-only.
    pub remote: Option<String>,
}

fn default_limit() -> i64 {
    20
}

#[derive(Serialize)]
struct JobListResponse {
    data: Vec<Job>,
    limit: i64,
    offset: i64,
    /// Matching jobs across all pages. Omitted by /jobs/mine.
    #[serde(skip_serializing_if = "Option::is_none")]
    total: Option<i64>,
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
    headers: HeaderMap,
    Query(q): Query<ListJobsQuery>,
) -> Result<Json<JobListResponse>, ApiError> {
    // The public board is published jobs only. Listing drafts or closed jobs
    // across every poster is a moderation view, so it needs a staff account;
    // posters see their own through /jobs/mine.
    if !matches!(q.status.as_deref(), None | Some("published")) {
        let account = require_account(&state, &headers).await?;
        require(&account, Action::Moderate, &Resource::Site).map_err(ApiError::from_domain)?;
    }
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
    let filters = jobshout_jobs::JobSearch {
        q: q.q.clone(),
        location: q.location.clone(),
        types: q
            .types
            .as_deref()
            .unwrap_or("")
            .split(',')
            .map(str::trim)
            .filter(|t| !t.is_empty())
            .map(str::to_string)
            .collect(),
        remote: matches!(q.remote.as_deref(), Some("1" | "true")),
    };
    let (data, total) = state
        .jobs
        .search(status, &filters, q.limit, q.offset)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(JobListResponse {
        data,
        limit: q.limit.clamp(1, 100),
        offset: q.offset.max(0),
        total: Some(total),
    }))
}

/// The signed-in poster's own jobs, drafts and closed ones included. The
/// public board only shows published jobs, so without this a draft had
/// nowhere to be seen.
async fn list_my_jobs(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<JobListResponse>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let data = state
        .jobs
        .list_mine(&account.primary_email)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(JobListResponse {
        data,
        limit: 100,
        offset: 0,
        total: None,
    }))
}

async fn publish_job(
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
        Action::UpdateJob,
        &Resource::Job {
            poster_email: poster,
            poster_account_id: None,
        },
    )
    .map_err(ApiError::from_domain)?;
    let job = state
        .jobs
        .publish(id)
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
    notify_job_alerts(&state, &job).await;
    Ok(Json(job))
}

/// Published jobs are public. Drafts and closed jobs are only visible to
/// whoever may edit them; everyone else gets a 404, so an unpublished role
/// cannot be read by guessing its id.
async fn get_job(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
) -> Result<Json<Job>, ApiError> {
    let job = state.jobs.get(id).await.map_err(ApiError::from_domain)?;
    if job.status == JobStatus::Published {
        return Ok(Json(job));
    }
    let account = state
        .accounts
        .resolve(&headers, state.internal_token.as_deref())
        .await
        .map_err(ApiError::from_domain)?;
    let Some(account) = account else {
        return Err(ApiError::from_domain(DomainError::NotFound));
    };
    let poster = state
        .jobs
        .poster_email(id)
        .await
        .map_err(ApiError::from_domain)?;
    let resource = Resource::Job {
        poster_email: poster,
        poster_account_id: None,
    };
    if require(&account, Action::UpdateJob, &resource).is_err() {
        return Err(ApiError::from_domain(DomainError::NotFound));
    }
    Ok(Json(job))
}

async fn delete_job(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
) -> Result<axum::http::StatusCode, ApiError> {
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
    state
        .jobs
        .delete_draft(id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(axum::http::StatusCode::NO_CONTENT)
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
    notify_job_alerts(&state, &job).await;
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

/* --- Saved jobs and job alerts ------------------------------------------ */

async fn list_saved(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<JobListResponse>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let data = state
        .saved
        .list(account.id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(JobListResponse {
        data,
        limit: 200,
        offset: 0,
        total: None,
    }))
}

async fn saved_state(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let saved = state
        .saved
        .is_saved(account.id, id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(serde_json::json!({ "saved": saved })))
}

async fn save_job(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let account = require_account(&state, &headers).await?;
    state
        .saved
        .save(account.id, id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(serde_json::json!({ "saved": true })))
}

async fn unsave_job(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
) -> Result<Json<serde_json::Value>, ApiError> {
    let account = require_account(&state, &headers).await?;
    state
        .saved
        .unsave(account.id, id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(serde_json::json!({ "saved": false })))
}

async fn list_alerts(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<serde_json::Value>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let data = state
        .saved
        .alerts(account.id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(serde_json::json!({ "data": data })))
}

async fn create_alert(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<jobshout_jobs::JobAlertInput>,
) -> Result<(axum::http::StatusCode, Json<jobshout_jobs::JobAlert>), ApiError> {
    let account = require_account(&state, &headers).await?;
    let alert = state
        .saved
        .create_alert(account.id, body)
        .await
        .map_err(ApiError::from_domain)?;
    Ok((axum::http::StatusCode::CREATED, Json(alert)))
}

async fn update_alert(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(alert_id): Path<Uuid>,
    Json(body): Json<jobshout_jobs::JobAlertInput>,
) -> Result<Json<jobshout_jobs::JobAlert>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let alert = state
        .saved
        .update_alert(account.id, alert_id, body)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(alert))
}

async fn delete_alert(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(alert_id): Path<Uuid>,
) -> Result<axum::http::StatusCode, ApiError> {
    let account = require_account(&state, &headers).await?;
    state
        .saved
        .delete_alert(account.id, alert_id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(axum::http::StatusCode::NO_CONTENT)
}

/// A job just went live: tell everyone whose alert matches (in-app).
async fn notify_job_alerts(state: &AppState, job: &Job) {
    if job.status != JobStatus::Published {
        return;
    }
    let recipients = match state.saved.alert_recipients(job.id).await {
        Ok(r) => r,
        Err(e) => {
            tracing::warn!(error = ?e, job_id = %job.id, "job alert matching failed");
            return;
        }
    };
    for email in recipients {
        state
            .notifications
            .notify_email(
                &email,
                "job_alert",
                &format!("New job matching your alert: {}", job.title),
                &job.summary,
                &format!("/jobs/{}", job.id),
                serde_json::json!({ "job_id": job.id }),
            )
            .await;
    }
}
