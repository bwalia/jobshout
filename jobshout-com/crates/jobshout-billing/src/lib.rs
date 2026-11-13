//! Credits ledger, bid costs, packs, subscriptions, Stripe webhook grants.
//!
//! Balance is always `SUM(amount)` for an account. Spend order: free →
//! subscription → purchased. Credits are granted from Stripe webhooks only
//! when purchases are enabled — never from a client "I paid" callback.

#![forbid(unsafe_code)]

use chrono::{DateTime, Datelike, Duration, NaiveDate, Utc};
use jobshout_domain::DomainError;
use rust_decimal::Decimal;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sqlx::PgPool;
use uuid::Uuid;

/// Free credits granted each calendar month when `CREDITS_ENABLED`.
pub const FREE_MONTHLY_CREDITS: i32 = 10;
/// Soft daily bid cap (proposals that spend credits).
pub const DAILY_BID_CAP: i32 = 20;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CreditKind {
    Grant,
    Purchase,
    Subscription,
    Bid,
    Refund,
    Expiry,
    Ai,
    Adjust,
}

impl CreditKind {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Grant => "grant",
            Self::Purchase => "purchase",
            Self::Subscription => "subscription",
            Self::Bid => "bid",
            Self::Refund => "refund",
            Self::Expiry => "expiry",
            Self::Ai => "ai",
            Self::Adjust => "adjust",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CreditBucket {
    Free,
    Subscription,
    Purchased,
}

impl CreditBucket {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Free => "free",
            Self::Subscription => "subscription",
            Self::Purchased => "purchased",
        }
    }

    fn parse(s: &str) -> Self {
        match s {
            "subscription" => Self::Subscription,
            "purchased" => Self::Purchased,
            _ => Self::Free,
        }
    }

    fn spend_order() -> [Self; 3] {
        [Self::Free, Self::Subscription, Self::Purchased]
    }
}

/// Bid cost by project budget band (GBP, using budget_max when set else budget_min).
pub fn bid_cost_for_budget(budget: Option<Decimal>) -> i32 {
    let Some(b) = budget else {
        return 2;
    };
    // Compare as i64 pounds (truncate).
    let pounds = b.to_string().parse::<f64>().unwrap_or(0.0);
    if pounds < 500.0 {
        1
    } else if pounds < 2_000.0 {
        2
    } else if pounds < 10_000.0 {
        4
    } else {
        6
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CreditBalance {
    pub account_id: Uuid,
    pub balance: i32,
    pub free: i32,
    pub subscription: i32,
    pub purchased: i32,
    pub expiring_soon: i32,
    pub plan: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LedgerEntry {
    pub id: Uuid,
    pub account_id: Uuid,
    pub kind: String,
    pub amount: i32,
    pub balance_after: i32,
    pub expires_at: Option<DateTime<Utc>>,
    pub bucket: String,
    pub idempotency_key: String,
    pub reference_type: String,
    pub reference_id: String,
    pub meta: Value,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CreditPack {
    pub id: Uuid,
    pub slug: String,
    pub name: String,
    pub credits: i32,
    pub price_cents: i32,
    pub currency: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SubscriptionPlan {
    pub id: Uuid,
    pub slug: String,
    pub name: String,
    pub monthly_credits: i32,
    pub seats: i32,
    pub price_cents: i32,
    pub currency: String,
    pub features: Value,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpendPreview {
    pub cost: i32,
    pub balance: i32,
    pub balance_after: i32,
    pub daily_bids: i32,
    pub daily_cap: i32,
}

struct LedgerWrite<'a> {
    kind: CreditKind,
    amount: i32,
    bucket: CreditBucket,
    idempotency_key: &'a str,
    reference_type: &'a str,
    reference_id: &'a str,
    expires_at: Option<DateTime<Utc>>,
    meta: Value,
}

#[derive(Clone)]
pub struct BillingService {
    pool: PgPool,
}

impl BillingService {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn balance(&self, account_id: Uuid) -> Result<CreditBalance, DomainError> {
        let rows: Vec<(String, i32, Option<DateTime<Utc>>)> = sqlx::query_as(
            r#"
            SELECT bucket, COALESCE(SUM(amount), 0)::int, MIN(expires_at)
            FROM credit_ledger
            WHERE account_id = $1
            GROUP BY bucket
            "#,
        )
        .bind(account_id)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        let mut free = 0;
        let mut subscription = 0;
        let mut purchased = 0;
        for (bucket, sum, _) in &rows {
            match CreditBucket::parse(bucket) {
                CreditBucket::Free => free = *sum,
                CreditBucket::Subscription => subscription = *sum,
                CreditBucket::Purchased => purchased = *sum,
            }
        }
        let balance = free + subscription + purchased;
        let soon = Utc::now() + Duration::days(14);
        let expiring_soon: (i32,) = sqlx::query_as(
            r#"
            SELECT COALESCE(SUM(amount), 0)::int
            FROM credit_ledger
            WHERE account_id = $1
              AND amount > 0
              AND expires_at IS NOT NULL
              AND expires_at <= $2
            "#,
        )
        .bind(account_id)
        .bind(soon)
        .fetch_one(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        let plan = self.active_plan_slug(account_id).await?;
        Ok(CreditBalance {
            account_id,
            balance,
            free: free.max(0),
            subscription: subscription.max(0),
            purchased: purchased.max(0),
            expiring_soon: expiring_soon.0.max(0),
            plan,
        })
    }

    async fn active_plan_slug(&self, account_id: Uuid) -> Result<String, DomainError> {
        let row: Option<(String,)> = sqlx::query_as(
            r#"
            SELECT p.slug
            FROM account_subscriptions s
            JOIN subscription_plans p ON p.id = s.plan_id
            WHERE s.account_id = $1 AND s.status IN ('active', 'trialing')
            ORDER BY s.created_at DESC
            LIMIT 1
            "#,
        )
        .bind(account_id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(row.map(|(s,)| s).unwrap_or_else(|| "free".into()))
    }

    /// Ensure the free monthly grant exists (idempotent per calendar month).
    pub async fn ensure_monthly_grant(&self, account_id: Uuid) -> Result<(), DomainError> {
        let now = Utc::now();
        let month_key = format!("grant:free:{}-{:02}", now.year(), now.month());
        let end = end_of_month(now);
        let period = format!("{}-{:02}", now.year(), now.month());
        let _ = self
            .append(
                account_id,
                LedgerWrite {
                    kind: CreditKind::Grant,
                    amount: FREE_MONTHLY_CREDITS,
                    bucket: CreditBucket::Free,
                    idempotency_key: &month_key,
                    reference_type: "monthly_grant",
                    reference_id: &period,
                    expires_at: Some(end),
                    meta: json!({ "credits": FREE_MONTHLY_CREDITS }),
                },
            )
            .await?;
        Ok(())
    }

    pub async fn preview_bid_spend(
        &self,
        account_id: Uuid,
        cost: i32,
    ) -> Result<SpendPreview, DomainError> {
        self.ensure_monthly_grant(account_id).await?;
        let bal = self.balance(account_id).await?;
        let daily = self.daily_bid_count(account_id).await?;
        Ok(SpendPreview {
            cost,
            balance: bal.balance,
            balance_after: bal.balance - cost,
            daily_bids: daily,
            daily_cap: DAILY_BID_CAP,
        })
    }

    pub async fn spend_for_bid(
        &self,
        account_id: Uuid,
        bid_id: Uuid,
        cost: i32,
    ) -> Result<i32, DomainError> {
        if cost <= 0 {
            return Ok(0);
        }
        self.ensure_monthly_grant(account_id).await?;
        let daily = self.daily_bid_count(account_id).await?;
        if daily >= DAILY_BID_CAP {
            return Err(DomainError::Validation(format!(
                "daily bid cap of {DAILY_BID_CAP} reached"
            )));
        }
        let bal = self.balance(account_id).await?;
        if bal.balance < cost {
            return Err(DomainError::Validation(format!(
                "not enough credits: need {cost}, have {}",
                bal.balance
            )));
        }

        let mut remaining = cost;
        let mut spent_total = 0;
        for bucket in CreditBucket::spend_order() {
            if remaining <= 0 {
                break;
            }
            let available = match bucket {
                CreditBucket::Free => bal.free,
                CreditBucket::Subscription => bal.subscription,
                CreditBucket::Purchased => bal.purchased,
            };
            if available <= 0 {
                continue;
            }
            let take = remaining.min(available);
            let key = format!("bid:{bid_id}:{}", bucket.as_str());
            let bid_ref = bid_id.to_string();
            self.append(
                account_id,
                LedgerWrite {
                    kind: CreditKind::Bid,
                    amount: -take,
                    bucket,
                    idempotency_key: &key,
                    reference_type: "bid",
                    reference_id: &bid_ref,
                    expires_at: None,
                    meta: json!({ "cost": cost, "bucket": bucket.as_str() }),
                },
            )
            .await?;
            remaining -= take;
            spent_total += take;
        }
        if remaining > 0 {
            return Err(DomainError::Validation(
                "not enough credits after allocation".into(),
            ));
        }
        self.bump_daily_bid(account_id).await?;
        Ok(spent_total)
    }

    pub async fn refund_bid(
        &self,
        account_id: Uuid,
        bid_id: Uuid,
        amount: i32,
        reason: &str,
    ) -> Result<(), DomainError> {
        if amount <= 0 {
            return Ok(());
        }
        let key = format!("refund:bid:{bid_id}:{reason}");
        let bid_ref = bid_id.to_string();
        // Refund into free bucket (simplest; purchased expiry not restored).
        let _ = self
            .append(
                account_id,
                LedgerWrite {
                    kind: CreditKind::Refund,
                    amount,
                    bucket: CreditBucket::Free,
                    idempotency_key: &key,
                    reference_type: "bid",
                    reference_id: &bid_ref,
                    expires_at: Some(Utc::now() + Duration::days(30)),
                    meta: json!({ "reason": reason, "amount": amount }),
                },
            )
            .await?;
        Ok(())
    }

    pub async fn meter_ai(
        &self,
        account_id: Uuid,
        draft_id: &str,
        cost: i32,
    ) -> Result<(), DomainError> {
        if cost <= 0 {
            return Ok(());
        }
        let key = format!("ai:{draft_id}");
        let bal = self.balance(account_id).await?;
        if bal.balance < cost {
            return Err(DomainError::Validation("not enough credits for AI".into()));
        }
        // Prefer subscription then purchased then free for AI metering.
        let order = [
            CreditBucket::Subscription,
            CreditBucket::Purchased,
            CreditBucket::Free,
        ];
        let mut remaining = cost;
        for bucket in order {
            if remaining <= 0 {
                break;
            }
            let available = match bucket {
                CreditBucket::Free => bal.free,
                CreditBucket::Subscription => bal.subscription,
                CreditBucket::Purchased => bal.purchased,
            };
            if available <= 0 {
                continue;
            }
            let take = remaining.min(available);
            let ikey = format!("{key}:{}", bucket.as_str());
            self.append(
                account_id,
                LedgerWrite {
                    kind: CreditKind::Ai,
                    amount: -take,
                    bucket,
                    idempotency_key: &ikey,
                    reference_type: "ai_draft",
                    reference_id: draft_id,
                    expires_at: None,
                    meta: json!({ "cost": cost }),
                },
            )
            .await?;
            remaining -= take;
        }
        Ok(())
    }

    /// Grant purchased credits — call ONLY from a verified Stripe webhook.
    pub async fn grant_purchase(
        &self,
        account_id: Uuid,
        credits: i32,
        stripe_event_id: &str,
        session_id: &str,
    ) -> Result<(), DomainError> {
        let key = format!("purchase:{stripe_event_id}");
        let expires = Utc::now() + Duration::days(365);
        let _ = self
            .append(
                account_id,
                LedgerWrite {
                    kind: CreditKind::Purchase,
                    amount: credits,
                    bucket: CreditBucket::Purchased,
                    idempotency_key: &key,
                    reference_type: "stripe_session",
                    reference_id: session_id,
                    expires_at: Some(expires),
                    meta: json!({ "stripe_event_id": stripe_event_id }),
                },
            )
            .await?;
        Ok(())
    }

    pub async fn grant_subscription_credits(
        &self,
        account_id: Uuid,
        credits: i32,
        period_key: &str,
        stripe_event_id: &str,
    ) -> Result<(), DomainError> {
        let key = format!("subscription:{period_key}:{stripe_event_id}");
        let expires = Utc::now() + Duration::days(35);
        let _ = self
            .append(
                account_id,
                LedgerWrite {
                    kind: CreditKind::Subscription,
                    amount: credits,
                    bucket: CreditBucket::Subscription,
                    idempotency_key: &key,
                    reference_type: "stripe_subscription",
                    reference_id: period_key,
                    expires_at: Some(expires),
                    meta: json!({ "stripe_event_id": stripe_event_id }),
                },
            )
            .await?;
        Ok(())
    }

    pub async fn list_packs(&self) -> Result<Vec<CreditPack>, DomainError> {
        let rows = sqlx::query_as::<_, (Uuid, String, String, i32, i32, String)>(
            r#"
            SELECT id, slug, name, credits, price_cents, currency
            FROM credit_packs
            WHERE active
            ORDER BY sort_order, credits
            "#,
        )
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows
            .into_iter()
            .map(
                |(id, slug, name, credits, price_cents, currency)| CreditPack {
                    id,
                    slug,
                    name,
                    credits,
                    price_cents,
                    currency,
                },
            )
            .collect())
    }

    pub async fn list_plans(&self) -> Result<Vec<SubscriptionPlan>, DomainError> {
        let rows = sqlx::query_as::<_, (Uuid, String, String, i32, i32, i32, String, Value)>(
            r#"
            SELECT id, slug, name, monthly_credits, seats, price_cents, currency, features
            FROM subscription_plans
            WHERE active
            ORDER BY sort_order
            "#,
        )
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows
            .into_iter()
            .map(
                |(id, slug, name, monthly_credits, seats, price_cents, currency, features)| {
                    SubscriptionPlan {
                        id,
                        slug,
                        name,
                        monthly_credits,
                        seats,
                        price_cents,
                        currency,
                        features,
                    }
                },
            )
            .collect())
    }

    pub async fn history(
        &self,
        account_id: Uuid,
        limit: i64,
    ) -> Result<Vec<LedgerEntry>, DomainError> {
        let limit = limit.clamp(1, 100);
        let rows = sqlx::query_as::<
            _,
            (
                Uuid,
                Uuid,
                String,
                i32,
                i32,
                Option<DateTime<Utc>>,
                String,
                String,
                String,
                String,
                Value,
                DateTime<Utc>,
            ),
        >(
            r#"
            SELECT id, account_id, kind, amount, balance_after, expires_at, bucket,
                   idempotency_key, reference_type, reference_id, meta, created_at
            FROM credit_ledger
            WHERE account_id = $1
            ORDER BY created_at DESC
            LIMIT $2
            "#,
        )
        .bind(account_id)
        .bind(limit)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows
            .into_iter()
            .map(
                |(
                    id,
                    account_id,
                    kind,
                    amount,
                    balance_after,
                    expires_at,
                    bucket,
                    idempotency_key,
                    reference_type,
                    reference_id,
                    meta,
                    created_at,
                )| LedgerEntry {
                    id,
                    account_id,
                    kind,
                    amount,
                    balance_after,
                    expires_at,
                    bucket,
                    idempotency_key,
                    reference_type,
                    reference_id,
                    meta,
                    created_at,
                },
            )
            .collect())
    }

    /// Record a Stripe webhook event idempotently. Returns false if already seen.
    pub async fn record_webhook_event(
        &self,
        event_id: &str,
        event_type: &str,
        payload: Value,
    ) -> Result<bool, DomainError> {
        let res = sqlx::query(
            r#"
            INSERT INTO stripe_webhook_events (id, type, payload)
            VALUES ($1, $2, $3)
            ON CONFLICT (id) DO NOTHING
            "#,
        )
        .bind(event_id)
        .bind(event_type)
        .bind(payload)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(res.rows_affected() > 0)
    }

    async fn append(
        &self,
        account_id: Uuid,
        entry: LedgerWrite<'_>,
    ) -> Result<Option<Uuid>, DomainError> {
        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| DomainError::Other(e.into()))?;

        let existing: Option<(Uuid,)> = sqlx::query_as(
            r#"SELECT id FROM credit_ledger WHERE account_id = $1 AND idempotency_key = $2"#,
        )
        .bind(account_id)
        .bind(entry.idempotency_key)
        .fetch_optional(&mut *tx)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        if existing.is_some() {
            tx.commit()
                .await
                .map_err(|e| DomainError::Other(e.into()))?;
            return Ok(None);
        }

        let (bal,): (i32,) = sqlx::query_as(
            r#"SELECT COALESCE(SUM(amount), 0)::int FROM credit_ledger WHERE account_id = $1"#,
        )
        .bind(account_id)
        .fetch_one(&mut *tx)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        let balance_after = bal + entry.amount;
        if balance_after < 0 {
            return Err(DomainError::Validation("insufficient credits".into()));
        }
        let id = Uuid::new_v4();
        sqlx::query(
            r#"
            INSERT INTO credit_ledger (
              id, account_id, kind, amount, balance_after, expires_at, bucket,
              idempotency_key, reference_type, reference_id, meta, created_at
            ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
            "#,
        )
        .bind(id)
        .bind(account_id)
        .bind(entry.kind.as_str())
        .bind(entry.amount)
        .bind(balance_after)
        .bind(entry.expires_at)
        .bind(entry.bucket.as_str())
        .bind(entry.idempotency_key)
        .bind(entry.reference_type)
        .bind(entry.reference_id)
        .bind(entry.meta)
        .execute(&mut *tx)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        tx.commit()
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        Ok(Some(id))
    }

    async fn daily_bid_count(&self, account_id: Uuid) -> Result<i32, DomainError> {
        let day = NaiveDate::from_ymd_opt(Utc::now().year(), Utc::now().month(), Utc::now().day())
            .unwrap_or_else(|| NaiveDate::from_ymd_opt(1970, 1, 1).unwrap());
        let row: Option<(i32,)> = sqlx::query_as(
            r#"SELECT bids_count FROM bid_daily_caps WHERE account_id = $1 AND day = $2"#,
        )
        .bind(account_id)
        .bind(day)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(row.map(|(c,)| c).unwrap_or(0))
    }

    async fn bump_daily_bid(&self, account_id: Uuid) -> Result<(), DomainError> {
        let day = NaiveDate::from_ymd_opt(Utc::now().year(), Utc::now().month(), Utc::now().day())
            .unwrap_or_else(|| NaiveDate::from_ymd_opt(1970, 1, 1).unwrap());
        sqlx::query(
            r#"
            INSERT INTO bid_daily_caps (account_id, day, bids_count)
            VALUES ($1, $2, 1)
            ON CONFLICT (account_id, day)
            DO UPDATE SET bids_count = bid_daily_caps.bids_count + 1
            "#,
        )
        .bind(account_id)
        .bind(day)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(())
    }
}

fn end_of_month(now: DateTime<Utc>) -> DateTime<Utc> {
    let (y, m) = if now.month() == 12 {
        (now.year() + 1, 1)
    } else {
        (now.year(), now.month() + 1)
    };
    NaiveDate::from_ymd_opt(y, m, 1)
        .and_then(|d| d.and_hms_opt(0, 0, 0))
        .map(|ndt| DateTime::<Utc>::from_naive_utc_and_offset(ndt, Utc) - Duration::seconds(1))
        .unwrap_or(now + Duration::days(30))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::str::FromStr;

    fn d(s: &str) -> Decimal {
        Decimal::from_str(s).expect("decimal")
    }

    #[test]
    fn bid_cost_bands() {
        assert_eq!(bid_cost_for_budget(None), 2);
        assert_eq!(bid_cost_for_budget(Some(d("100"))), 1);
        assert_eq!(bid_cost_for_budget(Some(d("500"))), 2);
        assert_eq!(bid_cost_for_budget(Some(d("1999"))), 2);
        assert_eq!(bid_cost_for_budget(Some(d("2000"))), 4);
        assert_eq!(bid_cost_for_budget(Some(d("10000"))), 6);
    }
}
