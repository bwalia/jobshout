use axum::extract::{Path, Query, State};
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_domain::{
    CandidateProfile, CandidateProfileId, JobMatch, UpsertCandidateProfileRequest,
};
use jobshout_matching::rank_jobs;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::error::ApiError;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/profiles", post(upsert_profile))
        .route("/api/v1/profiles/by-email", get(get_by_email))
        .route("/api/v1/profiles/{id}", get(get_profile))
        .route("/api/v1/profiles/{id}/matches", get(list_matches))
        .route(
            "/api/v1/profiles/{id}/matching-context",
            get(matching_context),
        )
}

#[derive(Debug, Deserialize)]
pub struct EmailQuery {
    pub email: String,
}

#[derive(Debug, Deserialize)]
pub struct MatchQuery {
    #[serde(default = "default_limit")]
    pub limit: i64,
}

fn default_limit() -> i64 {
    20
}

#[derive(Serialize)]
struct MatchListResponse {
    data: Vec<JobMatch>,
    limit: i64,
}

async fn upsert_profile(
    State(state): State<AppState>,
    Json(body): Json<UpsertCandidateProfileRequest>,
) -> Result<(axum::http::StatusCode, Json<CandidateProfile>), ApiError> {
    let profile = state
        .candidates
        .upsert(body)
        .await
        .map_err(ApiError::from_domain)?;
    Ok((axum::http::StatusCode::OK, Json(profile)))
}

async fn get_profile(
    State(state): State<AppState>,
    Path(id): Path<CandidateProfileId>,
) -> Result<Json<CandidateProfile>, ApiError> {
    let profile = state
        .candidates
        .get(id)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(profile))
}

async fn get_by_email(
    State(state): State<AppState>,
    Query(q): Query<EmailQuery>,
) -> Result<Json<CandidateProfile>, ApiError> {
    let profile = state
        .candidates
        .get_by_email(&q.email)
        .await
        .map_err(ApiError::from_domain)?;
    Ok(Json(profile))
}

async fn list_matches(
    State(state): State<AppState>,
    Path(id): Path<CandidateProfileId>,
    Query(q): Query<MatchQuery>,
) -> Result<Json<MatchListResponse>, ApiError> {
    let limit = q.limit.clamp(1, 50) as usize;
    let profile = state
        .candidates
        .get(id)
        .await
        .map_err(ApiError::from_domain)?;
    let jobs = state
        .jobs
        .list(100, 0, Some(jobshout_domain::JobStatus::Published))
        .await
        .map_err(ApiError::from_domain)?;
    let data = rank_jobs(&profile, &jobs, limit);
    Ok(Json(MatchListResponse {
        data,
        limit: limit as i64,
    }))
}

/// Payload Career / matching agents can load in one call.
async fn matching_context(
    State(state): State<AppState>,
    Path(id): Path<CandidateProfileId>,
    Query(q): Query<MatchQuery>,
) -> Result<Json<Value>, ApiError> {
    let limit = q.limit.clamp(1, 50) as usize;
    let profile = state
        .candidates
        .get(id)
        .await
        .map_err(ApiError::from_domain)?;
    let jobs = state
        .jobs
        .list(100, 0, Some(jobshout_domain::JobStatus::Published))
        .await
        .map_err(ApiError::from_domain)?;
    let matches = rank_jobs(&profile, &jobs, limit);
    Ok(Json(json!({
        "agent": "career_matching",
        "instruction": "Use the candidate profile and ranked matches to recommend roles. Always explain scores using the provided reasons. Never apply without human approval.",
        "profile": profile,
        "matches": matches,
    })))
}
