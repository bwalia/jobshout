//! HTTP privacy matrix for profiles / applications / job posting.
//!
//! Requires Postgres (same helper pattern as follows_pg). Skips unless
//! `SHOWCASE_TEST_DATABASE_URL` (or `DATABASE_URL`) is set.

use std::sync::Arc;

use axum::body::Body;
use axum::http::{Request, StatusCode};
use http_body_util::BodyExt;
use jobshout_api::{router, AppState};
use jobshout_applications::ApplicationService;
use jobshout_auth::AccountService;
use jobshout_candidates::CandidateService;
use jobshout_content::InsightService;
use jobshout_events::EventService;
use jobshout_freelance::FreelanceService;
use jobshout_jobs::{seed_organisation_id, JobService};
use jobshout_messaging::MessagingService;
use jobshout_notifications::NotificationService;
use jobshout_showcase::ShowcaseService;
use jobshout_users::MemberService;
use serde_json::Value;
use sqlx::PgPool;
use tower::ServiceExt;
use uuid::Uuid;

fn admin_url() -> Option<String> {
    std::env::var("SHOWCASE_TEST_DATABASE_URL")
        .ok()
        .or_else(|| std::env::var("DATABASE_URL").ok())
}

async fn scratch(admin_url: &str) -> (PgPool, String) {
    let name = format!("js_privacy_{}", Uuid::new_v4().simple());
    let admin = PgPool::connect(admin_url).await.expect("admin connect");
    sqlx::query(&format!("CREATE DATABASE \"{name}\""))
        .execute(&admin)
        .await
        .expect("create db");
    admin.close().await;
    let url = rewrite_db(admin_url, &name);
    let pool = jobshout_storage::connect(&url).await.expect("connect");
    jobshout_storage::migrate(&pool).await.expect("migrate");
    // Seed org required by jobs.
    sqlx::query(
        "INSERT INTO organisations (id, name, slug) VALUES ($1, 'Seed', 'seed') ON CONFLICT DO NOTHING",
    )
    .bind(seed_organisation_id())
    .execute(&pool)
    .await
    .ok();
    (pool, name)
}

async fn drop_db(admin_url: &str, pool: PgPool, name: &str) {
    pool.close().await;
    let admin = PgPool::connect(admin_url).await.expect("admin");
    sqlx::query(&format!("DROP DATABASE IF EXISTS \"{name}\" WITH (FORCE)"))
        .execute(&admin)
        .await
        .ok();
    admin.close().await;
}

fn rewrite_db(url: &str, name: &str) -> String {
    let (base, _) = url.rsplit_once('/').unwrap_or((url, ""));
    format!("{base}/{name}")
}

fn state(pool: PgPool) -> AppState {
    let token = Arc::<str>::from("test-internal-token");
    AppState {
        jobs: JobService::new(pool.clone(), seed_organisation_id()),
        candidates: CandidateService::new(pool.clone()),
        applications: ApplicationService::new(pool.clone()),
        insights: InsightService::new(pool.clone(), std::iter::empty::<String>()),
        showcase: ShowcaseService::new(pool.clone()),
        members: MemberService::new(pool.clone()),
        accounts: AccountService::new(pool.clone(), std::iter::empty::<String>()),
        events: EventService::new(pool.clone()),
        notifications: NotificationService::new(pool.clone()),
        messaging: MessagingService::new(pool.clone()),
        freelance: FreelanceService::new(pool),
        site_url: Arc::from("https://example.test"),
        internal_token: Some(token),
        email_outbox_enabled: false,
        members_rich_enabled: true,
        dashboard_enabled: true,
        messaging_enabled: true,
        projects_live: true,
    }
}

fn auth_headers(email: &str) -> axum::http::HeaderMap {
    let mut h = axum::http::HeaderMap::new();
    h.insert("x-jobshout-user-email", email.parse().unwrap());
    h.insert("x-jobshout-user-name", "Tester".parse().unwrap());
    h.insert(
        "x-jobshout-internal-token",
        "test-internal-token".parse().unwrap(),
    );
    h.insert("content-type", "application/json".parse().unwrap());
    h
}

async fn json_body(res: axum::response::Response) -> Value {
    let bytes = res.into_body().collect().await.unwrap().to_bytes();
    serde_json::from_slice(&bytes).unwrap_or(Value::Null)
}

#[tokio::test]
async fn unsigned_profile_and_applications_are_401() {
    let Some(admin) = admin_url() else {
        eprintln!("skip privacy_http: no DATABASE_URL");
        return;
    };
    let (pool, name) = scratch(&admin).await;
    let app = router(state(pool.clone()));

    for path in [
        "/api/v1/profiles/me",
        "/api/v1/applications",
        "/api/v1/jobs",
    ] {
        let method = if path == "/api/v1/jobs" {
            "POST"
        } else {
            "GET"
        };
        let req = Request::builder()
            .method(method)
            .uri(path)
            .header("content-type", "application/json")
            .body(if method == "POST" {
                Body::from(r#"{"title":"t","description":"long enough description here","employment_type":"permanent","location":{"country":"GB"}}"#)
            } else {
                Body::empty()
            })
            .unwrap();
        let res = app.clone().oneshot(req).await.unwrap();
        assert_eq!(
            res.status(),
            StatusCode::UNAUTHORIZED,
            "{method} {path} should be 401 without identity"
        );
    }

    drop_db(&admin, pool, &name).await;
}

#[tokio::test]
async fn owner_sees_pii_stranger_forbidden_on_profile() {
    let Some(admin) = admin_url() else {
        eprintln!("skip privacy_http: no DATABASE_URL");
        return;
    };
    let (pool, name) = scratch(&admin).await;
    let app = router(state(pool.clone()));

    let email = format!("{}@{}", "ada", "example.com");
    let upsert = Request::builder()
        .method("POST")
        .uri("/api/v1/profiles")
        .header("content-type", "application/json")
        .body(Body::from(format!(
            r#"{{"email":"{email}","display_name":"Ada","skills":["rust"],"cv_text":"SECRET_CV"}}"#
        )))
        .unwrap();
    let mut req = upsert;
    *req.headers_mut() = auth_headers(&email);
    let res = app.clone().oneshot(req).await.unwrap();
    assert_eq!(res.status(), StatusCode::OK);
    let body = json_body(res).await;
    assert_eq!(body["email"], email);
    assert_eq!(body["cv_text"], "SECRET_CV");
    let id = body["id"].as_str().unwrap().to_string();

    // Owner can read.
    let mut get = Request::builder()
        .method("GET")
        .uri(format!("/api/v1/profiles/{id}"))
        .body(Body::empty())
        .unwrap();
    *get.headers_mut() = auth_headers(&email);
    let res = app.clone().oneshot(get).await.unwrap();
    assert_eq!(res.status(), StatusCode::OK);
    let owner = json_body(res).await;
    assert!(owner.get("email").is_some());
    assert!(owner.get("cv_text").is_some());

    // Stranger is forbidden — response must not leak PII.
    let stranger = format!("{}@{}", "bob", "example.com");
    let mut get2 = Request::builder()
        .method("GET")
        .uri(format!("/api/v1/profiles/{id}"))
        .body(Body::empty())
        .unwrap();
    *get2.headers_mut() = auth_headers(&stranger);
    let res = app.clone().oneshot(get2).await.unwrap();
    assert_eq!(res.status(), StatusCode::FORBIDDEN);
    let denied = json_body(res).await;
    let dumped = denied.to_string();
    assert!(!dumped.contains("SECRET_CV"));
    assert!(!dumped.contains(&email));

    drop_db(&admin, pool, &name).await;
}

#[tokio::test]
async fn job_applications_only_for_poster() {
    let Some(admin) = admin_url() else {
        eprintln!("skip privacy_http: no DATABASE_URL");
        return;
    };
    let (pool, name) = scratch(&admin).await;
    let app = router(state(pool.clone()));

    let poster = format!("{}@{}", "hire", "co.com");
    let mut post = Request::builder()
        .method("POST")
        .uri("/api/v1/jobs")
        .body(Body::from(
            r#"{"title":"Rust eng","description":"Build the marketplace API in Rust and Axum.","employment_type":"permanent","location":{"country":"GB"},"publish":true}"#,
        ))
        .unwrap();
    *post.headers_mut() = auth_headers(&poster);
    let res = app.clone().oneshot(post).await.unwrap();
    let status = res.status();
    let job = json_body(res).await;
    assert_eq!(status, StatusCode::CREATED, "{job}");
    let job_id = job["id"].as_str().unwrap().to_string();

    let applicant = format!("{}@{}", "seek", "example.com");
    let phone = format!("{}{}", "+44", "7700900000");
    let mut apply = Request::builder()
        .method("POST")
        .uri(format!("/api/v1/jobs/{job_id}/applications"))
        .body(Body::from(format!(
            r#"{{"full_name":"Seek","email":"{applicant}","phone":"{phone}","cover_letter":"hello","cv_text":"CV_PRIVATE"}}"#
        )))
        .unwrap();
    *apply.headers_mut() = auth_headers(&applicant);
    let res = app.clone().oneshot(apply).await.unwrap();
    assert_eq!(res.status(), StatusCode::CREATED);

    // Poster can list (sees PII).
    let mut list = Request::builder()
        .method("GET")
        .uri(format!("/api/v1/jobs/{job_id}/applications"))
        .body(Body::empty())
        .unwrap();
    *list.headers_mut() = auth_headers(&poster);
    let res = app.clone().oneshot(list).await.unwrap();
    assert_eq!(res.status(), StatusCode::OK);
    let body = json_body(res).await;
    assert_eq!(body["data"][0]["email"], applicant);
    assert_eq!(body["data"][0]["phone"], phone);
    assert_eq!(body["data"][0]["cv_text"], "CV_PRIVATE");

    // Stranger cannot.
    let stranger = format!("{}@{}", "nosy", "example.com");
    let mut list2 = Request::builder()
        .method("GET")
        .uri(format!("/api/v1/jobs/{job_id}/applications"))
        .body(Body::empty())
        .unwrap();
    *list2.headers_mut() = auth_headers(&stranger);
    let res = app.clone().oneshot(list2).await.unwrap();
    assert_eq!(res.status(), StatusCode::FORBIDDEN);
    let denied = json_body(res).await.to_string();
    assert!(!denied.contains("CV_PRIVATE"));
    assert!(!denied.contains(&phone));

    drop_db(&admin, pool, &name).await;
}
