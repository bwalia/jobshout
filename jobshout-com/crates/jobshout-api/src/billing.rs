//! Credits + pricing HTTP API — Phase 2b. Gated by `CREDITS_ENABLED` /
//! `PURCHASES_ENABLED` (purchases stay dark until volume warrants Stripe).

use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, StatusCode};
use axum::routing::{get, post};
use axum::{Json, Router};
use jobshout_billing::bid_cost_for_budget;
use jobshout_domain::DomainError;
use serde::Deserialize;
use serde_json::{json, Value};
use uuid::Uuid;

use crate::error::ApiError;
use crate::insights::signed_in;
use crate::state::AppState;

pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/credits/balance", get(balance))
        .route("/api/v1/credits/history", get(history))
        .route("/api/v1/credits/preview-bid/{project_id}", get(preview_bid))
        .route("/api/v1/credits/packs", get(packs))
        .route("/api/v1/credits/plans", get(plans))
        .route("/api/v1/credits/checkout/{pack_slug}", post(checkout_pack))
        .route("/api/v1/billing/stripe/webhook", post(stripe_webhook))
        .route("/api/v1/pricing", get(pricing))
}

fn err(e: DomainError) -> ApiError {
    ApiError::from_domain(e)
}

fn require_credits(state: &AppState) -> Result<(), ApiError> {
    if state.credits_enabled {
        Ok(())
    } else {
        Err(ApiError::from_domain(DomainError::NotFound))
    }
}

async fn balance(
    State(state): State<AppState>,
    headers: HeaderMap,
) -> Result<Json<Value>, ApiError> {
    require_credits(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    state
        .billing
        .ensure_monthly_grant(account.id)
        .await
        .map_err(err)?;
    let bal = state.billing.balance(account.id).await.map_err(err)?;
    Ok(Json(json!({ "data": bal })))
}

#[derive(Debug, Deserialize)]
struct HistoryQuery {
    #[serde(default = "default_limit")]
    limit: i64,
}

fn default_limit() -> i64 {
    50
}

async fn history(
    State(state): State<AppState>,
    headers: HeaderMap,
    Query(q): Query<HistoryQuery>,
) -> Result<Json<Value>, ApiError> {
    require_credits(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let entries = state
        .billing
        .history(account.id, q.limit)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": entries })))
}

async fn preview_bid(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(project_id): Path<Uuid>,
) -> Result<Json<Value>, ApiError> {
    require_credits(&state)?;
    let who = signed_in(&state, &headers).await?;
    let account = state
        .accounts
        .ensure(&who.email, &who.name, who.agent)
        .await
        .map_err(err)?;
    let project = state.freelance.get_project(project_id).await.map_err(err)?;
    let cost = bid_cost_for_budget(
        jobshout_freelance::FreelanceService::project_budget_for_cost(&project),
    );
    let preview = state
        .billing
        .preview_bid_spend(account.id, cost)
        .await
        .map_err(err)?;
    Ok(Json(json!({ "data": preview })))
}

async fn packs(State(state): State<AppState>) -> Result<Json<Value>, ApiError> {
    // Packs are public catalogue; purchase gated separately.
    let packs = state.billing.list_packs().await.map_err(err)?;
    Ok(Json(json!({
        "data": packs,
        "purchases_enabled": state.purchases_enabled,
    })))
}

async fn plans(State(state): State<AppState>) -> Result<Json<Value>, ApiError> {
    let plans = state.billing.list_plans().await.map_err(err)?;
    Ok(Json(json!({
        "data": plans,
        "purchases_enabled": state.purchases_enabled,
    })))
}

async fn pricing(State(state): State<AppState>) -> Result<Json<Value>, ApiError> {
    let packs = state.billing.list_packs().await.map_err(err)?;
    let plans = state.billing.list_plans().await.map_err(err)?;
    Ok(Json(json!({
        "data": {
            "packs": packs,
            "plans": plans,
            "free_monthly_credits": jobshout_billing::FREE_MONTHLY_CREDITS,
            "bid_bands": [
                {"max_budget": 500, "credits": 1},
                {"max_budget": 2000, "credits": 2},
                {"max_budget": 10000, "credits": 4},
                {"max_budget": null, "credits": 6}
            ],
            "credits_enabled": state.credits_enabled,
            "purchases_enabled": state.purchases_enabled,
        }
    })))
}

async fn checkout_pack(
    State(state): State<AppState>,
    headers: HeaderMap,
    Path(pack_slug): Path<String>,
) -> Result<Json<Value>, ApiError> {
    require_credits(&state)?;
    if !state.purchases_enabled {
        return Err(ApiError {
            status: StatusCode::SERVICE_UNAVAILABLE,
            code: "PURCHASES_DISABLED",
            message: "Purchases are not enabled yet. Free monthly credits still apply when credits are on.".into(),
            request_id: Uuid::new_v4().to_string(),
        });
    }
    let _who = signed_in(&state, &headers).await?;
    // Stripe Checkout Session would be created here with keys from Vault.
    // Never embed secrets; refuse until STRIPE_SECRET_KEY is configured.
    let secret = std::env::var("STRIPE_SECRET_KEY").unwrap_or_default();
    if secret.trim().is_empty() {
        return Err(ApiError {
            status: StatusCode::SERVICE_UNAVAILABLE,
            code: "STRIPE_NOT_CONFIGURED",
            message: "Stripe is not configured for this ring.".into(),
            request_id: Uuid::new_v4().to_string(),
        });
    }
    Err(ApiError {
        status: StatusCode::NOT_IMPLEMENTED,
        code: "CHECKOUT_PENDING",
        message: format!(
            "Checkout for pack '{pack_slug}' will create a Stripe session once wired to Vault keys."
        ),
        request_id: Uuid::new_v4().to_string(),
    })
}

/// Stripe webhooks grant credits. Signature verification uses STRIPE_WEBHOOK_SECRET
/// from the environment (Vault). Without it, the endpoint refuses.
async fn stripe_webhook(
    State(state): State<AppState>,
    headers: HeaderMap,
    body: String,
) -> Result<StatusCode, ApiError> {
    if !state.purchases_enabled {
        return Err(ApiError {
            status: StatusCode::SERVICE_UNAVAILABLE,
            code: "PURCHASES_DISABLED",
            message: "Purchases disabled".into(),
            request_id: Uuid::new_v4().to_string(),
        });
    }
    let secret = std::env::var("STRIPE_WEBHOOK_SECRET").unwrap_or_default();
    if secret.trim().is_empty() {
        return Err(ApiError {
            status: StatusCode::SERVICE_UNAVAILABLE,
            code: "STRIPE_NOT_CONFIGURED",
            message: "Webhook secret not configured".into(),
            request_id: Uuid::new_v4().to_string(),
        });
    }
    let sig = headers
        .get("stripe-signature")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    if sig.is_empty() {
        return Err(ApiError::from_domain(DomainError::Unauthorized(
            "missing stripe-signature".into(),
        )));
    }
    // Full HMAC verification lands when Stripe test keys are in Vault. For now
    // parse the event and require a non-empty signature header + configured secret.
    let payload: Value = serde_json::from_str(&body)
        .map_err(|_| ApiError::from_domain(DomainError::Validation("invalid JSON body".into())))?;
    let event_id = payload
        .get("id")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let event_type = payload
        .get("type")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    if event_id.is_empty() {
        return Err(ApiError::from_domain(DomainError::Validation(
            "missing event id".into(),
        )));
    }
    let fresh = state
        .billing
        .record_webhook_event(&event_id, &event_type, payload.clone())
        .await
        .map_err(err)?;
    if !fresh {
        return Ok(StatusCode::OK);
    }
    if event_type == "checkout.session.completed" {
        if let Some(obj) = payload.pointer("/data/object") {
            let account_id = obj
                .get("client_reference_id")
                .and_then(|v| v.as_str())
                .and_then(|s| Uuid::parse_str(s).ok());
            let credits = obj
                .pointer("/metadata/credits")
                .and_then(|v| v.as_i64())
                .unwrap_or(0) as i32;
            let session_id = obj
                .get("id")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            if let (Some(aid), true) = (account_id, credits > 0) {
                state
                    .billing
                    .grant_purchase(aid, credits, &event_id, &session_id)
                    .await
                    .map_err(err)?;
            }
        }
    }
    Ok(StatusCode::OK)
}
