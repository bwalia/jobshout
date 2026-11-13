//! Policy engine: allow / deny for marketplace actions.
//!
//! Phase 0 ships a real [`can`] over account roles. Callers still enforce
//! ownership checks (email / account_id) where the resource is private.

#![forbid(unsafe_code)]

use jobshout_auth::{Account, ROLE_STAFF};
use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use uuid::Uuid;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Action {
    ReadProfile,
    WriteProfile,
    ReadOwnApplications,
    ApplyToJob,
    ReadJobApplications,
    PostJob,
    UpdateJob,
    CloseJob,
    Moderate,
    ReadMatchingContext,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Resource {
    /// Candidate profile owned by this account (or email during dual-read).
    CandidateProfile {
        owner_account_id: Option<Uuid>,
        owner_email: String,
    },
    /// Applications submitted by this email/account.
    OwnApplications { owner_email: String },
    /// Applications to a job — only the poster (or staff) may read.
    JobApplications {
        poster_email: String,
        poster_account_id: Option<Uuid>,
    },
    Job {
        poster_email: String,
        poster_account_id: Option<Uuid>,
    },
    /// Site-wide moderation / matching agents.
    Site,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Decision {
    Allow,
    Deny,
}

impl Decision {
    pub fn allowed(self) -> bool {
        matches!(self, Self::Allow)
    }
}

fn owns_email(account: &Account, email: &str) -> bool {
    account.email_eq(email)
}

fn owns_account(account: &Account, id: Option<Uuid>) -> bool {
    id.is_some_and(|id| id == account.id)
}

fn is_poster(account: &Account, poster_email: &str, poster_account_id: Option<Uuid>) -> bool {
    owns_account(account, poster_account_id) || owns_email(account, poster_email)
}

/// Decide whether `account` may perform `action` on `resource`.
pub fn can(account: &Account, action: Action, resource: &Resource) -> Decision {
    if account.is_staff() && matches!(action, Action::Moderate | Action::ReadMatchingContext) {
        return Decision::Allow;
    }
    if account.is_staff()
        && matches!(
            action,
            Action::ReadJobApplications | Action::UpdateJob | Action::CloseJob
        )
    {
        return Decision::Allow;
    }

    match (action, resource) {
        (
            Action::WriteProfile,
            Resource::CandidateProfile {
                owner_account_id,
                owner_email,
            },
        )
        | (
            Action::ReadProfile,
            Resource::CandidateProfile {
                owner_account_id,
                owner_email,
            },
        ) => {
            if owns_account(account, *owner_account_id) || owns_email(account, owner_email) {
                Decision::Allow
            } else {
                Decision::Deny
            }
        }
        (Action::ReadOwnApplications, Resource::OwnApplications { owner_email }) => {
            if owns_email(account, owner_email) {
                Decision::Allow
            } else {
                Decision::Deny
            }
        }
        (Action::ApplyToJob, Resource::Site) => {
            if account.agent {
                Decision::Deny
            } else {
                Decision::Allow
            }
        }
        (Action::PostJob, Resource::Site) => {
            if account.agent {
                Decision::Deny
            } else {
                Decision::Allow
            }
        }
        (
            Action::ReadJobApplications,
            Resource::JobApplications {
                poster_email,
                poster_account_id,
            },
        )
        | (
            Action::UpdateJob | Action::CloseJob,
            Resource::Job {
                poster_email,
                poster_account_id,
            },
        ) => {
            if is_poster(account, poster_email, *poster_account_id) {
                Decision::Allow
            } else {
                Decision::Deny
            }
        }
        (
            Action::ReadMatchingContext,
            Resource::CandidateProfile {
                owner_account_id,
                owner_email,
            },
        ) => {
            if account.agent
                || owns_account(account, *owner_account_id)
                || owns_email(account, owner_email)
            {
                Decision::Allow
            } else {
                Decision::Deny
            }
        }
        (Action::Moderate, Resource::Site) => {
            if account.roles.iter().any(|r| r == ROLE_STAFF) {
                Decision::Allow
            } else {
                Decision::Deny
            }
        }
        _ => Decision::Deny,
    }
}

/// Convenience: `can` or `Forbidden`.
pub fn require(account: &Account, action: Action, resource: &Resource) -> Result<(), DomainError> {
    if can(account, action, resource).allowed() {
        Ok(())
    } else {
        Err(DomainError::Forbidden("not allowed".into()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jobshout_auth::ROLE_MEMBER;

    fn account(email: &str, roles: &[&str]) -> Account {
        Account {
            id: Uuid::new_v4(),
            primary_email: email.into(),
            display_name: "T".into(),
            roles: roles.iter().map(|s| (*s).into()).collect(),
            agent: false,
        }
    }

    #[test]
    fn owner_reads_own_profile() {
        let a = account("ada@example.com", &[ROLE_MEMBER]);
        let r = Resource::CandidateProfile {
            owner_account_id: Some(a.id),
            owner_email: "ada@example.com".into(),
        };
        assert!(can(&a, Action::ReadProfile, &r).allowed());
    }

    #[test]
    fn stranger_denied_profile() {
        let a = account("bob@example.com", &[ROLE_MEMBER]);
        let r = Resource::CandidateProfile {
            owner_account_id: None,
            owner_email: "ada@example.com".into(),
        };
        assert!(!can(&a, Action::ReadProfile, &r).allowed());
    }

    #[test]
    fn poster_reads_applications() {
        let a = account("hire@co.com", &[ROLE_MEMBER]);
        let r = Resource::JobApplications {
            poster_email: "hire@co.com".into(),
            poster_account_id: None,
        };
        assert!(can(&a, Action::ReadJobApplications, &r).allowed());
    }

    #[test]
    fn staff_moderates() {
        let a = account("ed@jobshout.com", &[ROLE_MEMBER, ROLE_STAFF]);
        assert!(can(&a, Action::Moderate, &Resource::Site).allowed());
    }
}
