//! Linked Hire HTTP API — Phase 3. Gated by `HIRE_ENABLED`.

use axum::extract::{Path, State};
use axum::http::HeaderMap;
use axum::routing::get;
use axum::{Json, Router};
use jobshout_domain::DomainError;
use jobshout_hire::{CreateOrgRequest, CreateRequisitionRequest};
use serde_json::{json, Value};
use uuid::Uuid;

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/hire/orgs", get(list_orgs).post(create_org))
        .route(
            "/api/v1/hire/orgs/{org_id}/requisitions",
            get(list_requisitions).post(create_requisition),
        )
        .route(
            "/api/v1/hire/orgs/{org_id}/requisitions/{id}",
            get(get_requisition),
        )
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_hire(state: &AppState) -> Result<(), ApiError> {
    if state.hire_enabled {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

async fn list_orgs(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<Value>, ApiError> {
    require_hire(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let orgs = state
        .hire
        .list_orgs_for_account(account.id)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": orgs })))
}

async fn create_org(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(req): Json<CreateOrgRequest>,
) -> Result<Json<Value>, ApiError> {
    require_hire(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let org = state.hire.create_org(account.id, req).await.map_err(err)?;
    Ok(Json(json!({ "data": org })))
}

async fn list_requisitions(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(org_id): Path<Uuid>,
) -> Result<Json<Value>, ApiError> {
    require_hire(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let rows = state
        .hire
        .list_requisitions(org_id, account.id)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": rows })))
}

async fn create_requisition(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(org_id): Path<Uuid>,
    Json(req): Json<CreateRequisitionRequest>,
) -> Result<Json<Value>, ApiError> {
    require_hire(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let row = state
        .hire
        .create_requisition(org_id, account.id, req)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": row })))
}

async fn get_requisition(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path((org_id, id)): Path<(Uuid, Uuid)>,
) -> Result<Json<Value>, ApiError> {
    require_hire(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let row = state
        .hire
        .get_requisition_in_org(org_id, id, account.id)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": row })))
}
