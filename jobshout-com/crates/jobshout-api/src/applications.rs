use axum::extract::{Path, Query, State};
use axum::http::HeaderMap;
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::{
    ApplicationStatus, CreateJobApplicationRequest, JobApplication, JobApplicationId,
    JobApplicationWithJob, JobId,
};
use jobshout_policy::{require, Action, Resource};
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
        .route(
            "/api/v1/jobs/{id}/applications/{application_id}",
            axum::routing::patch(set_status),
        )
        .route("/api/v1/applications", get(list_mine))
        .route(
            "/api/v1/applications/{application_id}/withdraw",
            axum::routing::post(withdraw),
        )
}

#[derive(Debug, Deserialize)]
pub struct ListQuery {
    #[serde(default = "default_limit")]
    pub limit: i64,
    #[serde(default)]
    pub offset: i64,
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

async fn apply(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
    Json(mut body): Json<CreateJobApplicationRequest>,
) -> Result<(axum::http::StatusCode, Json<JobApplication>), ApiError> {
    let account = require_account(&state, &headers).await?;
    require(&account, Action::ApplyToJob, &Resource::Site).map_err(ApiError::from_domain)?;
    body.email = account.primary_email.clone();
    if body.full_name.trim().is_empty() {
        body.full_name = account.display_name.clone();
    }
    let application = state
        .applications
        .apply(id, body)
        .await
        .map_err(ApiError::from_domain)?;
    if let Ok(job) = state.jobs.get(id).await {
        if let Ok(poster) = state.jobs.poster_email(id).await {
            state
                .notifications
                .notify_email(
                    &poster,
                    "application_received",
                    &format!("New application: {}", job.title),
                    &format!("{} applied.", application.full_name),
                    &format!("/jobs/{id}/applicants#{}", application.id),
                    serde_json::json!({ "job_id": id, "application_id": application.id }),
                )
                .await;
        }
        let _ = state
            .events
            .emit(
                "job.application.submitted",
                "job",
                &id.to_string(),
                Some(account.id),
                serde_json::json!({
                    "application_id": application.id,
                    "job_title": job.title,
                }),
            )
            .await;
    }
    Ok((axum::http::StatusCode::CREATED, Json(application)))
}

async fn list_for_job(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<JobId>,
    Query(q): Query<ListQuery>,
) -> Result<Json<ApplicationListResponse>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let poster = state
        .jobs
        .poster_email(id)
        .await
        .map_err(ApiError::from_domain)?;
    require(
        &account,
        Action::ReadJobApplications,
        &Resource::JobApplications {
            poster_email: poster,
            poster_account_id: None,
        },
    )
    .map_err(ApiError::from_domain)?;
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
    headers: HeaderMap,
    Path(id): Path<JobId>,
) -> Result<Json<AppliedResponse>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let application = state
        .applications
        .find(id, &account.primary_email)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(AppliedResponse {
        applied: application.is_some(),
        application,
    }))
}

async fn list_mine(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(q): Query<ListQuery>,
) -> Result<Json<MyApplicationListResponse>, ApiError> {
    let account = require_account(&state, &headers).await?;
    require(
        &account,
        Action::ReadOwnApplications,
        &Resource::OwnApplications {
            owner_email: account.primary_email.clone(),
        },
    )
    .map_err(ApiError::from_domain)?;
    let data = state
        .applications
        .list_for_email(&account.primary_email, q.limit)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(MyApplicationListResponse {
        data,
        limit: q.limit.clamp(1, 100),
    }))
}

#[derive(Debug, Deserialize)]
pub struct SetStatusBody {
    pub status: ApplicationStatus,
}

/// The job's poster (or staff) moves an application through the pipeline.
/// The candidate sees the new status on /applications.
async fn set_status(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path((job_id, application_id)): Path<(JobId, JobApplicationId)>,
    Json(body): Json<SetStatusBody>,
) -> Result<Json<JobApplication>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let poster = state
        .jobs
        .poster_email(job_id)
        .await
        .map_err(ApiError::from_domain)?;
    require(
        &account,
        Action::ReadJobApplications,
        &Resource::JobApplications {
            poster_email: poster,
            poster_account_id: None,
        },
    )
    .map_err(ApiError::from_domain)?;
    let application = state
        .applications
        .set_status(job_id, application_id, body.status)
        .await
        .map_err(ApiError::from_domain)?;
    let title = state
        .jobs
        .get(job_id)
        .await
        .map(|j| j.title)
        .unwrap_or_else(|_| "your application".into());
    state
        .notifications
        .notify_email(
            &application.email,
            "application_status",
            &format!("Update on {title}"),
            &format!(
                "Your application is now: {}.",
                status_label(application.status)
            ),
            "/applications",
            serde_json::json!({ "job_id": job_id, "application_id": application.id }),
        )
        .await;
    let _ = state
        .events
        .emit(
            "job.application.status_changed",
            "job_application",
            &application.id.to_string(),
            Some(account.id),
            serde_json::json!({
                "job_id": job_id,
                "status": application.status,
                "candidate_email": application.email,
            }),
        )
        .await;
    Ok(Json(application))
}

/// The candidate withdraws their own application. Anyone else's
/// application id answers 404.
async fn withdraw(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(application_id): Path<JobApplicationId>,
) -> Result<Json<JobApplication>, ApiError> {
    let account = require_account(&state, &headers).await?;
    let application = state
        .applications
        .withdraw(application_id, &account.primary_email)
        .await
        .map_err(ApiError::from_domain)?;
    let _ = state
        .events
        .emit(
            "job.application.withdrawn",
            "job_application",
            &application.id.to_string(),
            Some(account.id),
            serde_json::json!({ "job_id": application.job_id }),
        )
        .await;
    Ok(Json(application))
}

/// How a status reads to the candidate (matches the web's labels).
fn status_label(s: ApplicationStatus) -> &'static str {
    match s {
        ApplicationStatus::Submitted => "Submitted",
        ApplicationStatus::Reviewing => "In review",
        ApplicationStatus::Shortlisted => "Shortlisted",
        ApplicationStatus::Interviewing => "Interviewing",
        ApplicationStatus::Offered => "Offer",
        ApplicationStatus::Rejected => "Not moving forward",
        ApplicationStatus::Withdrawn => "Withdrawn",
    }
}
