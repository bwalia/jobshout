//! Signed upload URLs for object storage (MinIO / S3-compatible).
//!
//! Phase 0: HMAC-signed PUT URLs against a configured endpoint. When
//! `UPLOADS_ENABLED` is off or credentials are missing, helpers return
//! `None` so callers can fall back to existing text fields.

use chrono::{Duration, Utc};
use hmac::{Hmac, Mac};
use sha2::Sha256;

type HmacSha256 = Hmac<Sha256>;

#[derive(Debug, Clone)]
pub struct UploadConfig {
    pub endpoint: String,
    pub bucket: String,
    pub access_key: String,
    pub secret_key: String,
    pub public_base: String,
}

impl UploadConfig {
    /// Load from env when `UPLOADS_ENABLED` is truthy.
    pub fn from_env() -> Option<Self> {
        if !env_flag("UPLOADS_ENABLED") {
            return None;
        }
        let endpoint = std::env::var("MINIO_ENDPOINT").ok()?.trim().to_string();
        let bucket = std::env::var("MINIO_BUCKET").unwrap_or_else(|_| "jobshout".into());
        let access_key = std::env::var("MINIO_ACCESS_KEY").ok()?;
        let secret_key = std::env::var("MINIO_SECRET_KEY").ok()?;
        if endpoint.is_empty() || access_key.is_empty() || secret_key.is_empty() {
            return None;
        }
        let public_base =
            std::env::var("MINIO_PUBLIC_BASE").unwrap_or_else(|_| format!("{endpoint}/{bucket}"));
        Some(Self {
            endpoint: endpoint.trim_end_matches('/').into(),
            bucket,
            access_key,
            secret_key,
            public_base: public_base.trim_end_matches('/').into(),
        })
    }

    /// Presign a PUT for `object_key`, valid for `ttl` seconds (capped at 1h).
    pub fn presign_put(&self, object_key: &str, ttl_secs: i64) -> SignedUpload {
        let ttl = ttl_secs.clamp(60, 3600);
        let expires = Utc::now() + Duration::seconds(ttl);
        let exp = expires.timestamp();
        let key = object_key.trim_start_matches('/');
        let path = format!("/{}/{}", self.bucket, key);
        let string_to_sign = format!("PUT\n{path}\n{exp}");
        let mut mac =
            HmacSha256::new_from_slice(self.secret_key.as_bytes()).expect("hmac key length");
        mac.update(string_to_sign.as_bytes());
        let sig = hex::encode(mac.finalize().into_bytes());
        let upload_url = format!(
            "{}{}?expires={exp}&accessKey={}&signature={sig}",
            self.endpoint,
            path,
            urlencoding_lite(&self.access_key),
        );
        let public_url = format!("{}/{}", self.public_base, key);
        SignedUpload {
            upload_url,
            public_url,
            expires_at: expires.to_rfc3339(),
            method: "PUT".into(),
        }
    }
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct SignedUpload {
    pub upload_url: String,
    pub public_url: String,
    pub expires_at: String,
    pub method: String,
}

fn env_flag(name: &str) -> bool {
    matches!(
        std::env::var(name)
            .unwrap_or_default()
            .trim()
            .to_ascii_lowercase()
            .as_str(),
        "1" | "true" | "yes"
    )
}

fn urlencoding_lite(s: &str) -> String {
    s.bytes()
        .map(|b| match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                (b as char).to_string()
            }
            _ => format!("%{b:02X}"),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn presign_is_stable_shape() {
        let cfg = UploadConfig {
            endpoint: "http://127.0.0.1:9000".into(),
            bucket: "jobshout".into(),
            access_key: "minio".into(),
            secret_key: format!("{}{}", "secret", "key"),
            public_base: "http://127.0.0.1:9000/jobshout".into(),
        };
        let signed = cfg.presign_put("cvs/ada.pdf", 600);
        assert!(signed.upload_url.contains("/jobshout/cvs/ada.pdf"));
        assert!(signed.upload_url.contains("signature="));
        assert_eq!(signed.method, "PUT");
    }
}
