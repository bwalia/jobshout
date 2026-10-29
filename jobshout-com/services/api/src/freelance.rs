//! HTTP handlers for the freelancer marketplace

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::IntoResponse,
    Json,
};
use jobshout_domain::{
    BidStatus, CreateBidRequest, CreateProjectRequest, DomainError, ProjectType,
    UpsertFreelancerProfileRequest,
};
use jobshout_freelance::FreelanceService;
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::collections::HashMap;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ListProjectsQuery {
    #[serde(default = "default_limit")]
    limit: i64,
    #[serde(default)]
    offset: i64,
    #[serde(default)]
    project_type: Option<ProjectType>,
    #[serde(default)]
    category_ids: Option<Vec<Uuid>>,
}

fn default_limit() -> i64 {
    20
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ListBidsQuery {
    #[serde(default = "default_limit")]
    limit: i64,
    #[serde(default)]
    offset: i64,
    #[serde(default)]
    status: Option<BidStatus>,
}

/// List all projects
pub async fn list_projects(
    State(service): State<FreelanceService>,
    Query(query): Query<ListProjectsQuery>,
) -> impl IntoResponse {
    match service
        .list_projects(query.limit, query.offset, query.project_type, query.category_ids)
        .await
    {
        Ok(projects) => (StatusCode::OK, Json(json!({ "projects": projects }))),
        Err(e) => handle_domain_error(e),
    }
}

/// Create a new project
pub async fn create_project(
    State(service): State<FreelanceService>,
    Json(req): Json<CreateProjectRequest>,
) -> impl IntoResponse {
    // TODO: Get client_id from authenticated session
    let client_id = Uuid::new_v4(); // Placeholder

    match service.create_project(req, client_id).await {
        Ok(project) => (StatusCode::CREATED, Json(json!({ "project": project }))),
        Err(e) => handle_domain_error(e),
    }
}

/// Get a specific project
pub async fn get_project(
    State(service): State<FreelanceService>,
    Path(project_id): Path<Uuid>,
) -> impl IntoResponse {
    match service.get_project(project_id).await {
        Ok(project) => (StatusCode::OK, Json(json!({ "project": project }))),
        Err(DomainError::NotFound) => (
            StatusCode::NOT_FOUND,
            Json(json!({ "error": "Project not found" })),
        ),
        Err(e) => handle_domain_error(e),
    }
}

/// List bids for a project
pub async fn list_project_bids(
    State(service): State<FreelanceService>,
    Path(project_id): Path<Uuid>,
    Query(query): Query<ListBidsQuery>,
) -> impl IntoResponse {
    match service
        .list_bids(project_id, query.limit, query.offset, query.status)
        .await
    {
        Ok(bids) => (StatusCode::OK, Json(json!({ "bids": bids }))),
        Err(e) => handle_domain_error(e),
    }
}

/// Submit a bid on a project
pub async fn submit_bid(
    State(service): State<FreelanceService>,
    Path(project_id): Path<Uuid>,
    Json(req): Json<CreateBidRequest>,
) -> impl IntoResponse {
    // TODO: Get freelancer_id from authenticated session
    let freelancer_id = Uuid::new_v4(); // Placeholder

    match service.submit_bid(project_id, req, freelancer_id).await {
        Ok(bid) => (StatusCode::CREATED, Json(json!({ "bid": bid }))),
        Err(e) => handle_domain_error(e),
    }
}

/// List my bids (freelancer view)
pub async fn list_my_bids(
    State(service): State<FreelanceService>,
    Query(query): Query<ListBidsQuery>,
) -> impl IntoResponse {
    // TODO: Get freelancer_id from authenticated session
    let freelancer_id = Uuid::new_v4(); // Placeholder

    match service
        .list_my_bids(freelancer_id, query.limit, query.offset, query.status)
        .await
    {
        Ok(bids) => (StatusCode::OK, Json(json!({ "bids": bids }))),
        Err(e) => handle_domain_error(e),
    }
}

/// Update bid status (client view)
pub async fn update_bid_status(
    State(service): State<FreelanceService>,
    Path(bid_id): Path<Uuid>,
    Json(body): Json<HashMap<String, String>>,
) -> impl IntoResponse {
    let status_str = body.get("status").map(|s| s.as_str()).unwrap_or("");
    let status = match BidStatus::parse(status_str) {
        Some(s) => s,
        None => {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({ "error": "Invalid status value" })),
            );
        }
    };

    // TODO: Get client_id from authenticated session
    let client_id = Uuid::new_v4(); // Placeholder

    match service.update_bid_status(bid_id, status, client_id).await {
        Ok(bid) => (StatusCode::OK, Json(json!({ "bid": bid }))),
        Err(e) => handle_domain_error(e),
    }
}

/// Get my freelancer profile
pub async fn get_my_freelancer_profile(
    State(service): State<FreelanceService>,
) -> impl IntoResponse {
    // TODO: Get user_id from authenticated session
    let user_id = Uuid::new_v4(); // Placeholder

    match service.get_freelancer_profile(user_id).await {
        Ok(Some(profile)) => (StatusCode::OK, Json(json!({ "profile": profile }))),
        Ok(None) => (
            StatusCode::NOT_FOUND,
            Json(json!({ "error": "Freelancer profile not found" })),
        ),
        Err(e) => handle_domain_error(e),
    }
}

/// Create or update freelancer profile
pub async fn upsert_freelancer_profile(
    State(service): State<FreelanceService>,
    Json(req): Json<UpsertFreelancerProfileRequest>,
) -> impl IntoResponse {
    // TODO: Get user_id from authenticated session
    let user_id = Uuid::new_v4(); // Placeholder

    match service.upsert_freelancer_profile(user_id, req).await {
        Ok(profile) => (StatusCode::OK, Json(json!({ "profile": profile }))),
        Err(e) => handle_domain_error(e),
    }
}

/// List project categories
pub async fn list_categories(
    State(service): State<FreelanceService>,
) -> impl IntoResponse {
    match service.list_categories().await {
        Ok(categories) => (StatusCode::OK, Json(json!({ "categories": categories }))),
        Err(e) => handle_domain_error(e),
    }
}

/// Handle domain errors and convert to HTTP responses
fn handle_domain_error(err: DomainError) -> (StatusCode, axum::Json<serde_json::Value>) {
    match err {
        DomainError::NotFound => (
            StatusCode::NOT_FOUND,
            Json(json!({ "error": "Resource not found" })),
        ),
        DomainError::Validation(msg) => (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": format!("Validation error: {}", msg) })),
        ),
        DomainError::Conflict(msg) => (
            StatusCode::CONFLICT,
            Json(json!({ "error": format!("Conflict: {}", msg) })),
        ),
        DomainError::Unauthorized(msg) => (
            StatusCode::UNAUTHORIZED,
            Json(json!({ "error": format!("Unauthorized: {}", msg) })),
        ),
        DomainError::Forbidden(msg) => (
            StatusCode::FORBIDDEN,
            Json(json!({ "error": format!("Forbidden: {}", msg) })),
        ),
        DomainError::RateLimited(msg) => (
            StatusCode::TOO_MANY_REQUESTS,
            Json(json!({ "error": format!("Rate limited: {}", msg) })),
        ),
        DomainError::Other(_) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": "Internal server error" })),
        ),
    }
}