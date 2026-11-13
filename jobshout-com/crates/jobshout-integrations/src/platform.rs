//! Thin client for the JobShout Go platform `POST /api/v1/tasks/launch`.
//! Marketplace AI must call this — never invent a second AI runtime.

use jobshout_domain::DomainError;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::collections::HashMap;

#[derive(Clone)]
pub struct PlatformClient {
    base_url: String,
    api_key: String,
    project_id: String,
    agent_ids: HashMap<String, String>,
    http: reqwest::Client,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LaunchResult {
    pub message: String,
    pub draft: String,
    pub task_id: String,
    pub run_id: String,
    pub raw: Value,
}

impl PlatformClient {
    /// Build from env. Returns None when URL or key is unset (AI stays dark / unavailable).
    pub fn from_env() -> Option<Self> {
        let base_url = std::env::var("JOBSHOUT_PLATFORM_URL")
            .ok()
            .map(|s| s.trim().trim_end_matches('/').to_string())
            .filter(|s| !s.is_empty())?;
        let api_key = std::env::var("JOBSHOUT_PLATFORM_API_KEY")
            .ok()
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())?;
        let project_id = std::env::var("JOBSHOUT_PLATFORM_PROJECT_ID").unwrap_or_default();
        let mut agent_ids = HashMap::new();
        for (env, builtin) in [
            ("JOBSHOUT_PLATFORM_AGENT_PROPOSAL", "proposal_generator"),
            ("JOBSHOUT_PLATFORM_AGENT_BREAKDOWN", "project_breakdown"),
            ("JOBSHOUT_PLATFORM_AGENT_TALENT", "talent_matchmaker"),
        ] {
            if let Ok(v) = std::env::var(env) {
                let v = v.trim().to_string();
                if !v.is_empty() {
                    agent_ids.insert(builtin.into(), v);
                }
            }
        }
        Some(Self {
            base_url,
            api_key,
            project_id,
            agent_ids,
            http: reqwest::Client::new(),
        })
    }

    pub fn agent_id(&self, builtin: &str) -> Option<&str> {
        self.agent_ids.get(builtin).map(String::as_str)
    }

    pub async fn launch(
        &self,
        agent_id: &str,
        values: HashMap<String, String>,
    ) -> Result<LaunchResult, DomainError> {
        if self.project_id.trim().is_empty() {
            return Err(DomainError::Validation(
                "JOBSHOUT_PLATFORM_PROJECT_ID is required to launch AI tasks".into(),
            ));
        }
        let url = format!("{}/api/v1/tasks/launch", self.base_url);
        let body = json!({
            "agent_id": agent_id,
            "project_id": self.project_id,
            "values": values,
        });
        let res = self
            .http
            .post(&url)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await
            .map_err(|e| DomainError::Other(e.into()))?;
        let status = res.status();
        let raw: Value = res.json().await.map_err(|e| DomainError::Other(e.into()))?;
        if !status.is_success() {
            let msg = raw
                .pointer("/error")
                .and_then(|v| v.as_str())
                .or_else(|| raw.pointer("/message").and_then(|v| v.as_str()))
                .unwrap_or("platform launch failed");
            return Err(DomainError::Validation(format!(
                "platform launch {status}: {msg}"
            )));
        }
        let draft = raw
            .pointer("/task/metadata/draft")
            .and_then(|v| v.as_str())
            .or_else(|| raw.pointer("/message").and_then(|v| v.as_str()))
            .unwrap_or("")
            .to_string();
        let task_id = raw
            .pointer("/task/id")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let run_id = raw
            .pointer("/run_id")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let message = raw
            .pointer("/message")
            .and_then(|v| v.as_str())
            .unwrap_or("AI draft ready")
            .to_string();
        Ok(LaunchResult {
            message,
            draft,
            task_id,
            run_id,
            raw,
        })
    }
}
