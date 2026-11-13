//! AI draft endpoints — Phase 5a. Gated by `AI_FEATURES`.
//! Drafts come from the Go platform (`POST /tasks/launch`); humans approve.

use axum::extract::{Path, State};
use axum::http::HeaderMap;
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_domain::DomainError;
use serde::Deserialize;
use serde_json::{json, Value};
use sqlx::PgPool;
use std::collections::HashMap;
use uuid::Uuid;

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/ai/drafts", get(list_drafts))
        .route("/api/v1/ai/drafts/{id}/approve", post(approve_draft))
        .route("/api/v1/ai/drafts/{id}/discard", post(discard_draft))
        .route("/api/v1/ai/proposal", post(generate_proposal))
        .route("/api/v1/ai/project-breakdown", post(generate_breakdown))
        .route("/api/v1/ai/talent-match", post(generate_talent_match))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_ai(state: &AppState) -> Result<(), ApiError> {
    if state.ai_features {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

fn pool(state: &AppState) -> Option<&PgPool> {
    state.ai_pool.as_ref()
}

#[allow(clippy::too_many_arguments)]
async fn persist_draft(
    pool: &PgPool,
    account_id: Uuid,
    kind: &str,
    title: &str,
    body: &str,
    input: Value,
    task_id: &str,
    run_id: &str,
) -> Result<Uuid, DomainError> {
    let id = Uuid::new_v4();
    sqlx::query(
        r#"
        INSERT INTO ai_drafts (
          id, account_id, kind, status, title, body, input,
          platform_task_id, platform_run_id, created_at, updated_at
        ) VALUES ($1,$2,$3,'pending',$4,$5,$6,$7,$8,NOW(),NOW())
        "#,
    )
    .bind(id)
    .bind(account_id)
    .bind(kind)
    .bind(title)
    .bind(body)
    .bind(input)
    .bind(task_id)
    .bind(run_id)
    .execute(pool)
    .await
    .map_err(|e| DomainError::Other(e.into()))?;
    Ok(id)
}

#[derive(Debug, Deserialize)]
struct ProposalBody {
    project_title: String,
    project_description: String,
    #[serde(default)]
    skills: String,
    #[serde(default)]
    freelancer_profile: String,
    #[serde(default)]
    tone: String,
}

async fn generate_proposal(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<ProposalBody>,
) -> Result<Json<Value>, ApiError> {
    require_ai(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let client = state.platform.as_ref().ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation(
            "JOBSHOUT_PLATFORM_URL / API key not configured".into(),
        ))
    })?;
    let agent_id = client.agent_id("proposal_generator").ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation(
            "JOBSHOUT_PLATFORM_AGENT_PROPOSAL not set".into(),
        ))
    })?;
    let mut values = HashMap::new();
    values.insert("project_title".into(), body.project_title.clone());
    values.insert(
        "project_description".into(),
        body.project_description.clone(),
    );
    values.insert("skills".into(), body.skills.clone());
    values.insert("freelancer_profile".into(), body.freelancer_profile.clone());
    values.insert("tone".into(), body.tone.clone());
    let launched = client.launch(agent_id, values).await.map_err(err)?;
    let pool = pool(&state).ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation("ai storage not configured".into()))
    })?;
    let id = persist_draft(
        pool,
        account.id,
        "proposal",
        &body.project_title,
        &launched.draft,
        json!({
            "project_title": body.project_title,
            "project_description": body.project_description,
        }),
        &launched.task_id,
        &launched.run_id,
    )
    .await
    .map_err(err)?;
    if state.credits_enabled {
        let _ = state.billing.meter_ai(account.id, &id.to_string(), 1).await;
    }
    Ok(Json(json!({
        "data": {
            "id": id,
            "kind": "proposal",
            "body": launched.draft,
            "message": launched.message,
            "platform_task_id": launched.task_id,
        }
    })))
}

#[derive(Debug, Deserialize)]
struct BreakdownBody {
    idea: String,
    #[serde(default)]
    budget_hint: String,
    #[serde(default)]
    preferred_stack: String,
}

async fn generate_breakdown(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<BreakdownBody>,
) -> Result<Json<Value>, ApiError> {
    require_ai(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let client = state.platform.as_ref().ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation(
            "JOBSHOUT_PLATFORM_URL / API key not configured".into(),
        ))
    })?;
    let agent_id = client.agent_id("project_breakdown").ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation(
            "JOBSHOUT_PLATFORM_AGENT_BREAKDOWN not set".into(),
        ))
    })?;
    let mut values = HashMap::new();
    values.insert("idea".into(), body.idea.clone());
    values.insert("budget_hint".into(), body.budget_hint.clone());
    values.insert("preferred_stack".into(), body.preferred_stack.clone());
    let launched = client.launch(agent_id, values).await.map_err(err)?;
    let pool = pool(&state).ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation("ai storage not configured".into()))
    })?;
    let id = persist_draft(
        pool,
        account.id,
        "project_breakdown",
        "Project breakdown",
        &launched.draft,
        json!({ "idea": body.idea }),
        &launched.task_id,
        &launched.run_id,
    )
    .await
    .map_err(err)?;
    Ok(Json(json!({
        "data": {
            "id": id,
            "kind": "project_breakdown",
            "body": launched.draft,
            "message": launched.message,
            "platform_task_id": launched.task_id,
        }
    })))
}

#[derive(Debug, Deserialize)]
struct TalentBody {
    brief: String,
    #[serde(default)]
    required_skills: String,
    #[serde(default)]
    candidate_notes: String,
}

async fn generate_talent_match(
    State(state): State<AppState>,
    headers: HeaderMap,
    Json(body): Json<TalentBody>,
) -> Result<Json<Value>, ApiError> {
    require_ai(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let client = state.platform.as_ref().ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation(
            "JOBSHOUT_PLATFORM_URL / API key not configured".into(),
        ))
    })?;
    let agent_id = client.agent_id("talent_matchmaker").ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation(
            "JOBSHOUT_PLATFORM_AGENT_TALENT not set".into(),
        ))
    })?;
    let mut values = HashMap::new();
    values.insert("brief".into(), body.brief.clone());
    values.insert("required_skills".into(), body.required_skills.clone());
    values.insert("candidate_notes".into(), body.candidate_notes.clone());
    let launched = client.launch(agent_id, values).await.map_err(err)?;
    let pool = pool(&state).ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation("ai storage not configured".into()))
    })?;
    let id = persist_draft(
        pool,
        account.id,
        "talent_match",
        "Talent match",
        &launched.draft,
        json!({ "brief": body.brief }),
        &launched.task_id,
        &launched.run_id,
    )
    .await
    .map_err(err)?;
    Ok(Json(json!({
        "data": {
            "id": id,
            "kind": "talent_match",
            "body": launched.draft,
            "message": launched.message,
            "platform_task_id": launched.task_id,
        }
    })))
}

async fn list_drafts(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<Value>, ApiError> {
    require_ai(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let pool = pool(&state).ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation("ai storage not configured".into()))
    })?;
    let rows = sqlx::query_as::<
        _,
        (
            Uuid,
            String,
            String,
            String,
            String,
            chrono::DateTime<chrono::Utc>,
        ),
    >(
        r#"
        SELECT id, kind, status, title, body, created_at
        FROM ai_drafts
        WHERE account_id = $1
        ORDER BY created_at DESC
        LIMIT 50
        "#,
    )
    .bind(account.id)
    .fetch_all(pool)
    .await
    .map_err(|e| err(DomainError::Other(e.into())))?;
    let data: Vec<Value> = rows
        .into_iter()
        .map(|(id, kind, status, title, body, created_at)| {
            json!({
                "id": id,
                "kind": kind,
                "status": status,
                "title": title,
                "body": body,
                "created_at": created_at,
            })
        })
        .collect();
    Ok(Json(json!({ "data": data })))
}

async fn approve_draft(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<Uuid>,
) -> Result<Json<Value>, ApiError> {
    set_status(&state, &headers, id, "approved").await
}

async fn discard_draft(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(id): Path<Uuid>,
) -> Result<Json<Value>, ApiError> {
    set_status(&state, &headers, id, "discarded").await
}

async fn set_status(
    state: &AppState,
    headers: &HeaderMap,
    id: Uuid,
    status: &str,
) -> Result<Json<Value>, ApiError> {
    require_ai(state)?;
    let who = signed_in(state, headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let pool = pool(state).ok_or_else(|| {
        ApiError::from_domain(DomainError::Validation("ai storage not configured".into()))
    })?;
    let res = sqlx::query(
        r#"
        UPDATE ai_drafts SET status = $3, updated_at = NOW()
        WHERE id = $1 AND account_id = $2
        "#,
    )
    .bind(id)
    .bind(account.id)
    .bind(status)
    .execute(pool)
    .await
    .map_err(|e| err(DomainError::Other(e.into())))?;
    if res.rows_affected() == 0 {
        return Err(ApiError::from_domain(DomainError::NotFound));
    }
    Ok(Json(json!({ "data": { "id": id, "status": status } })))
}
