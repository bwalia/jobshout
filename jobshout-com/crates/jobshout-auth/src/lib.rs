//! Authentication and identity for JobShout.com.
//!
//! The web tier forwards the signed-in user as `x-jobshout-user-*` headers,
//! signed with `x-jobshout-internal-token`. This crate resolves those claims
//! into an [`Account`] with roles.

#![forbid(unsafe_code)]

use std::collections::HashSet;
use std::sync::Arc;

use chrono::Utc;
use http::HeaderMap;
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use uuid::Uuid;

/// Well-known account roles (Phase 0).
pub const ROLE_MEMBER: &str = "member";
pub const ROLE_FREELANCER: &str = "freelancer";
pub const ROLE_JOB_SEEKER: &str = "job_seeker";
pub const ROLE_RECRUITER: &str = "recruiter";
pub const ROLE_EMPLOYER: &str = "employer";
pub const ROLE_AGENCY: &str = "agency";
pub const ROLE_AGENT_OWNER: &str = "agent_owner";
pub const ROLE_STAFF: &str = "staff";

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Account {
    pub id: Uuid,
    pub primary_email: String,
    pub display_name: String,
    pub roles: Vec<String>,
    /// Platform agent acting under a synthetic address.
    pub agent: bool,
}

impl Account {
    pub fn is_staff(&self) -> bool {
        self.roles.iter().any(|r| r == ROLE_STAFF)
    }

    pub fn has_role(&self, role: &str) -> bool {
        self.roles.iter().any(|r| r == role)
    }

    pub fn email_eq(&self, other: &str) -> bool {
        self.primary_email.eq_ignore_ascii_case(other.trim())
    }
}

fn same(a: &[u8], b: &[u8]) -> bool {
    a.len() == b.len() && a.iter().zip(b).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}

fn header_text(headers: &HeaderMap, name: &str) -> String {
    headers
        .get(name)
        .and_then(|v| v.to_str().ok())
        .map(str::trim)
        .unwrap_or_default()
        .to_string()
}

/// Raw identity claim extracted from signed headers (before DB resolve).
#[derive(Debug, Clone)]
pub struct IdentityClaim {
    pub email: String,
    pub name: String,
    pub agent: bool,
}

/// Extract a signed identity claim from request headers.
///
/// Returns `Ok(None)` when no identity headers are present. Returns
/// `Unauthorized` when identity headers are present but the internal token
/// does not match (unless `internal_token` is `None` for local unsigned mode).
pub fn extract_claim(
    headers: &HeaderMap,
    internal_token: Option<&str>,
) -> Result<Option<IdentityClaim>, DomainError> {
    let email = header_text(headers, "x-jobshout-user-email");
    let agent = header_text(headers, "x-jobshout-agent");
    if email.is_empty() && agent.is_empty() {
        return Ok(None);
    }
    if let Some(expected) = internal_token {
        if !same(
            header_text(headers, "x-jobshout-internal-token").as_bytes(),
            expected.as_bytes(),
        ) {
            return Err(DomainError::Unauthorized(
                "untrusted identity headers".into(),
            ));
        }
    }
    if !agent.is_empty() {
        let slug = agent
            .trim()
            .to_ascii_lowercase()
            .chars()
            .map(|c| if c.is_ascii_alphanumeric() { c } else { '-' })
            .collect::<String>();
        return Ok(Some(IdentityClaim {
            email: format!("{slug}@agents.jobshout.com"),
            name: format!("JobShout {}", agent.trim()),
            agent: true,
        }));
    }
    Ok(Some(IdentityClaim {
        email: email.to_ascii_lowercase(),
        name: header_text(headers, "x-jobshout-user-name"),
        agent: false,
    }))
}

#[derive(Clone)]
pub struct AccountService {
    pool: PgPool,
    /// Bootstrap staff emails (INSIGHTS_STAFF_EMAILS). Synced into account_roles
    /// on ensure; kept so existing Helm values still work.
    bootstrap_staff: Arc<HashSet<String>>,
}

impl AccountService {
    pub fn new(pool: PgPool, bootstrap_staff: impl IntoIterator<Item = String>) -> Self {
        Self {
            pool,
            bootstrap_staff: Arc::new(
                bootstrap_staff
                    .into_iter()
                    .map(|e| e.trim().to_ascii_lowercase())
                    .filter(|e| !e.is_empty())
                    .collect(),
            ),
        }
    }

    /// Resolve headers → account (creating one if needed). Agents get accounts too.
    pub async fn resolve(
        &self,
        headers: &HeaderMap,
        internal_token: Option<&str>,
    ) -> Result<Option<Account>, DomainError> {
        let Some(claim) = extract_claim(headers, internal_token)? else {
            return Ok(None);
        };
        Ok(Some(
            self.ensure(&claim.email, &claim.name, claim.agent).await?,
        ))
    }

    pub async fn require(
        &self,
        headers: &HeaderMap,
        internal_token: Option<&str>,
    ) -> Result<Account, DomainError> {
        self.resolve(headers, internal_token)
            .await?
            .ok_or_else(|| DomainError::Unauthorized("sign in to do that".into()))
    }

    /// Find or create an account for `email`, grant member (+ staff if bootstrap).
    pub async fn ensure(
        &self,
        email: &str,
        display_name: &str,
        agent: bool,
    ) -> Result<Account, DomainError> {
        let email = email.trim().to_ascii_lowercase();
        if email.is_empty() || !email.contains('@') {
            return Err(DomainError::Validation("email is required".into()));
        }
        let name = display_name.trim();
        let existing = sqlx::query_as::<_, (Uuid, String, String)>(
            r#"
            SELECT id, primary_email, display_name
            FROM accounts
            WHERE lower(primary_email) = $1
            "#,
        )
        .bind(&email)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;

        let id = if let Some((id, _, stored_name)) = existing {
            if !name.is_empty() && stored_name.trim().is_empty() {
                let _ = sqlx::query(
                    "UPDATE accounts SET display_name = $2, updated_at = NOW() WHERE id = $1",
                )
                .bind(id)
                .bind(name)
                .execute(&self.pool)
                .await;
            }
            id
        } else {
            let id = Uuid::new_v4();
            let display = if name.is_empty() {
                email.split('@').next().unwrap_or("member").to_string()
            } else {
                name.to_string()
            };
            let insert = sqlx::query(
                r#"
                INSERT INTO accounts (id, primary_email, display_name, created_at, updated_at)
                VALUES ($1, $2, $3, NOW(), NOW())
                "#,
            )
            .bind(id)
            .bind(&email)
            .bind(&display)
            .execute(&self.pool)
            .await;
            match insert {
                Ok(_) => {}
                Err(e) if matches!(e.as_database_error().and_then(|d| d.code()), Some(c) if c == "23505") =>
                {
                    // Concurrent insert won; fall through to re-select.
                }
                Err(e) => return Err(DomainError::Other(e.into())),
            }
            // Race: another insert may have won; re-select.
            let (id, _, _): (Uuid, String, String) = sqlx::query_as(
                "SELECT id, primary_email, display_name FROM accounts WHERE lower(primary_email) = $1",
            )
            .bind(&email)
            .fetch_one(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
            let _ = sqlx::query(
                r#"
                INSERT INTO account_identities (id, account_id, provider, provider_subject, email)
                VALUES ($1, $2, 'email', $3, $3)
                ON CONFLICT (provider, provider_subject) DO NOTHING
                "#,
            )
            .bind(Uuid::new_v4())
            .bind(id)
            .bind(&email)
            .execute(&self.pool)
            .await;
            id
        };

        self.grant_role(id, ROLE_MEMBER).await?;
        if !agent && self.bootstrap_staff.contains(&email) {
            self.grant_role(id, ROLE_STAFF).await?;
        }
        if agent {
            self.grant_role(id, ROLE_AGENT_OWNER).await?;
        }

        // Link member / candidate_profile rows when present.
        let _ = sqlx::query(
            "UPDATE members SET account_id = $1 WHERE account_id IS NULL AND email = $2",
        )
        .bind(id)
        .bind(&email)
        .execute(&self.pool)
        .await;
        let _ = sqlx::query(
            "UPDATE candidate_profiles SET account_id = $1 WHERE account_id IS NULL AND lower(email) = $2",
        )
        .bind(id)
        .bind(&email)
        .execute(&self.pool)
        .await;

        self.load(id, agent).await
    }

    pub async fn grant_role(&self, account_id: Uuid, role: &str) -> Result<(), DomainError> {
        sqlx::query(
            r#"
            INSERT INTO account_roles (account_id, role, granted_at)
            VALUES ($1, $2, $3)
            ON CONFLICT DO NOTHING
            "#,
        )
        .bind(account_id)
        .bind(role)
        .bind(Utc::now())
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(())
    }

    async fn load(&self, id: Uuid, agent: bool) -> Result<Account, DomainError> {
        let (primary_email, display_name): (String, String) =
            sqlx::query_as("SELECT primary_email, display_name FROM accounts WHERE id = $1")
                .bind(id)
                .fetch_optional(&self.pool)
                .await
                .map_err(|e| DomainError::Other(e.into()))?
                .ok_or(DomainError::NotFound)?;
        let roles: Vec<String> = sqlx::query_scalar(
            "SELECT role FROM account_roles WHERE account_id = $1 ORDER BY role",
        )
        .bind(id)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(Account {
            id,
            primary_email,
            display_name,
            roles,
            agent,
        })
    }

    /// Bootstrap staff role for every email in the env allowlist (idempotent).
    pub async fn sync_bootstrap_staff(&self) -> Result<usize, DomainError> {
        let mut n = 0;
        for email in self.bootstrap_staff.iter() {
            let account = self.ensure(email, "", false).await?;
            if !account.is_staff() {
                self.grant_role(account.id, ROLE_STAFF).await?;
                n += 1;
            } else {
                n += 1;
            }
        }
        Ok(n)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use http::HeaderMap;

    #[test]
    fn extract_claim_none_without_headers() {
        let headers = HeaderMap::new();
        assert!(extract_claim(&headers, Some("tok")).unwrap().is_none());
    }

    #[test]
    fn extract_claim_rejects_bad_token() {
        let mut headers = HeaderMap::new();
        headers.insert("x-jobshout-user-email", "a@b.co".parse().unwrap());
        headers.insert("x-jobshout-internal-token", "wrong".parse().unwrap());
        let err = extract_claim(&headers, Some("tok")).unwrap_err();
        assert!(matches!(err, DomainError::Unauthorized(_)));
    }

    #[test]
    fn extract_claim_accepts_matching_token() {
        let mut headers = HeaderMap::new();
        headers.insert("x-jobshout-user-email", "Ada@Example.com".parse().unwrap());
        headers.insert("x-jobshout-user-name", "Ada".parse().unwrap());
        headers.insert("x-jobshout-internal-token", "tok".parse().unwrap());
        let claim = extract_claim(&headers, Some("tok")).unwrap().unwrap();
        assert_eq!(claim.email, "ada@example.com");
        assert_eq!(claim.name, "Ada");
        assert!(!claim.agent);
    }
}
