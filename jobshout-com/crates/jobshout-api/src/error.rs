use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use jobshout_domain::{ApiErrorBody, ApiErrorDetail, DomainError};
use uuid::Uuid;

pub struct ApiError {
    pub status: StatusCode,
    pub code: &'static str,
    pub message: String,
    pub request_id: String,
}

impl ApiError {
    pub fn from_domain(err: DomainError) -> Self {
        let request_id = Uuid::new_v4().to_string();
        match err {
            DomainError::NotFound => Self {
                status: StatusCode::NOT_FOUND,
                code: "NOT_FOUND",
                message: "Resource not found".into(),
                request_id,
            },
            DomainError::Validation(msg) => Self {
                status: StatusCode::BAD_REQUEST,
                code: "VALIDATION_ERROR",
                message: msg,
                request_id,
            },
            DomainError::Conflict(msg) => Self {
                status: StatusCode::CONFLICT,
                code: "CONFLICT",
                message: msg,
                request_id,
            },
            DomainError::Other(e) => {
                tracing::error!(error = %e, "internal error");
                Self {
                    status: StatusCode::INTERNAL_SERVER_ERROR,
                    code: "INTERNAL_ERROR",
                    message: "Internal server error".into(),
                    request_id,
                }
            }
        }
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        let body = ApiErrorBody {
            error: ApiErrorDetail {
                code: self.code.to_string(),
                message: self.message,
                request_id: self.request_id,
            },
        };
        (self.status, Json(body)).into_response()
    }
}
