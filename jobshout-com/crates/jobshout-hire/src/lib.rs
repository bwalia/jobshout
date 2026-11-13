//! Linked Hire — organisations, requisitions, shortlists and invitations.
//! Every query is org-scoped. Contact details stay hidden until invite accept.

#![forbid(unsafe_code)]

use chrono::{DateTime, Utc};
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HireOrg {
    pub id: Uuid,
    pub name: String,
    pub slug: String,
    pub owner_account_id: Option<Uuid>,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Requisition {
    pub id: Uuid,
    pub org_id: Uuid,
    pub title: String,
    pub description: String,
    pub kind: String,
    pub status: String,
    pub location: String,
    pub remote_ok: bool,
    pub skills: Vec<String>,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CreateOrgRequest {
    pub name: String,
    pub slug: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CreateRequisitionRequest {
    pub title: String,
    #[serde(default)]
    pub description: String,
    #[serde(default = "default_kind")]
    pub kind: String,
    #[serde(default)]
    pub location: String,
    #[serde(default = "default_true")]
    pub remote_ok: bool,
    #[serde(default)]
    pub skills: Vec<String>,
}

fn default_kind() -> String {
    "permanent".into()
}
fn default_true() -> bool {
    true
}

#[derive(Clone)]
pub struct HireService {
    pool: PgPool,
}

impl HireService {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub async fn create_org(
        &self,
        account_id: Uuid,
        req: CreateOrgRequest,
    ) -> Result<HireOrg, DomainError> {
        let name = req.name.trim();
        let slug = req.slug.trim().to_lowercase();
        if name.is_empty() || slug.is_empty() {
            return Err(DomainError::Validation("name and slug are required".into()));
        }
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO hire_organisations (id, name, slug, owner_account_id, created_at, updated_at)
            VALUES ($1,$2,$3,$4,$5,$5)
            "#,
        )
        .bind(id)
        .bind(name)
        .bind(&slug)
        .bind(account_id)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        sqlx::query(
            r#"
            INSERT INTO hire_org_members (id, org_id, account_id, role, created_at)
            VALUES ($1,$2,$3,'owner',NOW())
            "#,
        )
        .bind(Uuid::new_v4())
        .bind(id)
        .bind(account_id)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        // Default pipeline stages for the org.
        for (i, stage) in ["Applied", "Screen", "Interview", "Offer", "Hired"]
            .iter()
            .enumerate()
        {
            sqlx::query(
                r#"
                INSERT INTO hire_pipeline_stages (id, org_id, name, position)
                VALUES ($1,$2,$3,$4)
                "#,
            )
            .bind(Uuid::new_v4())
            .bind(id)
            .bind(*stage)
            .bind(i as i32)
            .execute(&self.pool)
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        }
        Ok(HireOrg {
            id,
            name: name.into(),
            slug,
            owner_account_id: Some(account_id),
            created_at: now,
        })
    }

    pub async fn list_orgs_for_account(
        &self,
        account_id: Uuid,
    ) -> Result<Vec<HireOrg>, DomainError> {
        let rows = sqlx::query_as::<_, (Uuid, String, String, Option<Uuid>, DateTime<Utc>)>(
            r#"
            SELECT o.id, o.name, o.slug, o.owner_account_id, o.created_at
            FROM hire_organisations o
            JOIN hire_org_members m ON m.org_id = o.id
            WHERE m.account_id = $1
            ORDER BY o.created_at DESC
            "#,
        )
        .bind(account_id)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows
            .into_iter()
            .map(|(id, name, slug, owner_account_id, created_at)| HireOrg {
                id,
                name,
                slug,
                owner_account_id,
                created_at,
            })
            .collect())
    }

    async fn require_member(&self, org_id: Uuid, account_id: Uuid) -> Result<String, DomainError> {
        let row: Option<(String,)> = sqlx::query_as(
            r#"SELECT role FROM hire_org_members WHERE org_id = $1 AND account_id = $2"#,
        )
        .bind(org_id)
        .bind(account_id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        row.map(|(r,)| r).ok_or_else(|| {
            DomainError::Forbidden("not a member of this hiring organisation".into())
        })
    }

    pub async fn create_requisition(
        &self,
        org_id: Uuid,
        account_id: Uuid,
        req: CreateRequisitionRequest,
    ) -> Result<Requisition, DomainError> {
        self.require_member(org_id, account_id).await?;
        let title = req.title.trim();
        if title.is_empty() {
            return Err(DomainError::Validation("title is required".into()));
        }
        let kind = match req.kind.as_str() {
            "permanent" | "contract" | "freelance" => req.kind.as_str(),
            _ => "permanent",
        };
        let id = Uuid::new_v4();
        let now = Utc::now();
        sqlx::query(
            r#"
            INSERT INTO hire_requisitions (
              id, org_id, title, description, kind, status, location, remote_ok,
              skills, created_by, created_at, updated_at
            ) VALUES ($1,$2,$3,$4,$5,'draft',$6,$7,$8,$9,$10,$10)
            "#,
        )
        .bind(id)
        .bind(org_id)
        .bind(title)
        .bind(&req.description)
        .bind(kind)
        .bind(&req.location)
        .bind(req.remote_ok)
        .bind(&req.skills)
        .bind(account_id)
        .bind(now)
        .execute(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(Requisition {
            id,
            org_id,
            title: title.into(),
            description: req.description,
            kind: kind.into(),
            status: "draft".into(),
            location: req.location,
            remote_ok: req.remote_ok,
            skills: req.skills,
            created_at: now,
        })
    }

    pub async fn list_requisitions(
        &self,
        org_id: Uuid,
        account_id: Uuid,
    ) -> Result<Vec<Requisition>, DomainError> {
        self.require_member(org_id, account_id).await?;
        let rows = sqlx::query_as::<
            _,
            (
                Uuid,
                Uuid,
                String,
                String,
                String,
                String,
                String,
                bool,
                Vec<String>,
                DateTime<Utc>,
            ),
        >(
            r#"
            SELECT id, org_id, title, description, kind, status, location, remote_ok, skills, created_at
            FROM hire_requisitions
            WHERE org_id = $1
            ORDER BY created_at DESC
            LIMIT 100
            "#,
        )
        .bind(org_id)
        .fetch_all(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        Ok(rows
            .into_iter()
            .map(
                |(
                    id,
                    org_id,
                    title,
                    description,
                    kind,
                    status,
                    location,
                    remote_ok,
                    skills,
                    created_at,
                )| Requisition {
                    id,
                    org_id,
                    title,
                    description,
                    kind,
                    status,
                    location,
                    remote_ok,
                    skills,
                    created_at,
                },
            )
            .collect())
    }

    /// Two-org isolation helper used by tests: requisitions for org A never
    /// appear when querying as a member of org B only.
    pub async fn get_requisition_in_org(
        &self,
        org_id: Uuid,
        requisition_id: Uuid,
        account_id: Uuid,
    ) -> Result<Requisition, DomainError> {
        self.require_member(org_id, account_id).await?;
        let row = sqlx::query_as::<
            _,
            (
                Uuid,
                Uuid,
                String,
                String,
                String,
                String,
                String,
                bool,
                Vec<String>,
                DateTime<Utc>,
            ),
        >(
            r#"
            SELECT id, org_id, title, description, kind, status, location, remote_ok, skills, created_at
            FROM hire_requisitions
            WHERE id = $1 AND org_id = $2
            "#,
        )
        .bind(requisition_id)
        .bind(org_id)
        .fetch_optional(&self.pool)
        .await
        .map_err(|e| DomainError::Other(e.into()))?;
        let Some((
            id,
            org_id,
            title,
            description,
            kind,
            status,
            location,
            remote_ok,
            skills,
            created_at,
        )) = row
        else {
            return Err(DomainError::NotFound);
        };
        Ok(Requisition {
            id,
            org_id,
            title,
            description,
            kind,
            status,
            location,
            remote_ok,
            skills,
            created_at,
        })
    }
}
