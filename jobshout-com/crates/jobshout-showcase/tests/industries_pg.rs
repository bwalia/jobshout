//! Industry filtering against a real Postgres.
//!
//! Skipped unless SHOWCASE_TEST_DATABASE_URL points at a server this test
//! may create and drop a scratch database on, e.g.
//! `postgres://postgres@127.0.0.1:5432/postgres`.

use jobshout_content::Actor;
use jobshout_domain::{
    ShowcaseAppInput, ShowcaseAppType, ShowcaseBuildMethod, ShowcaseKind, ShowcaseMaturity,
    ShowcasePricing,
};
use jobshout_showcase::{ListQuery, ShowcaseService};
use sqlx::{Connection, Executor, PgConnection, PgPool};

fn editor() -> Actor {
    Actor {
        email: "editors@example.com".into(),
        name: "Editors".into(),
        is_staff: true,
        agent: false,
    }
}

fn entry(name: &str, industries: &[&str]) -> ShowcaseAppInput {
    ShowcaseAppInput {
        name: name.into(),
        tagline: format!("{name} tagline"),
        description_md: "A test entry for the industry filter. It exists only inside a scratch database that this test creates, fills, checks and then drops again.".into(),
        app_type: Some(ShowcaseAppType::AiApplication),
        maturity: Some(ShowcaseMaturity::Beta),
        build_method: Some(ShowcaseBuildMethod::HumanAi),
        pricing: Some(ShowcasePricing::Free),
        repo_url: "https://example.com/source".into(),
        industries: Some(industries.iter().map(|s| s.to_string()).collect()),
        submit: true,
        ..Default::default()
    }
}

/// A scratch database on the test server, migrated. Dropped by [`drop_db`].
async fn scratch(admin_url: &str) -> (PgPool, String) {
    let name = format!("showcase_ind_{}", uuid::Uuid::new_v4().simple());
    let mut admin = PgConnection::connect(admin_url).await.expect("connect");
    admin
        .execute(format!("CREATE DATABASE {name}").as_str())
        .await
        .expect("create database");
    let base = admin_url
        .rsplit_once('/')
        .map(|(b, _)| b)
        .unwrap_or(admin_url);
    let pool = jobshout_storage::connect(&format!("{base}/{name}"))
        .await
        .expect("connect scratch");
    jobshout_storage::migrate(&pool).await.expect("migrate");
    (pool, name)
}

async fn drop_db(admin_url: &str, pool: PgPool, name: &str) {
    pool.close().await;
    if let Ok(mut admin) = PgConnection::connect(admin_url).await {
        let _ = admin
            .execute(format!("DROP DATABASE IF EXISTS {name} WITH (FORCE)").as_str())
            .await;
    }
}

fn names(apps: &[jobshout_domain::ShowcaseApp]) -> Vec<String> {
    apps.iter().map(|a| a.name.clone()).collect()
}

#[tokio::test]
async fn industry_filter_counts_and_migration_replay() {
    let Ok(admin_url) = std::env::var("SHOWCASE_TEST_DATABASE_URL") else {
        eprintln!("SHOWCASE_TEST_DATABASE_URL unset; skipping");
        return;
    };
    let (pool, db) = scratch(&admin_url).await;
    let svc = ShowcaseService::new(pool.clone());
    let ed = editor();

    let conveyancing = svc
        .create(&ed, entry("Conveyancing Tool", &["conveyancing"]))
        .await
        .expect("create conveyancing");
    // The parent is stored with the vertical, parents first.
    assert_eq!(conveyancing.industries, vec!["legal", "conveyancing"]);
    svc.create(&ed, entry("Legal Only", &["legal"]))
        .await
        .expect("create legal");
    svc.create(&ed, entry("General Tool", &["cross-industry"]))
        .await
        .expect("create general");
    let mut draft = entry("Legal Draft", &["legal"]);
    draft.submit = false;
    svc.create(&ed, draft).await.expect("create draft");

    let list = |industry: &str, cross: bool| ListQuery {
        industry: Some(industry.into()),
        include_cross_industry: cross,
        limit: 50,
        ..Default::default()
    };

    // An industry finds entries tagged with any of its verticals.
    let (legal, total) = svc.list_published(list("legal", false)).await.unwrap();
    assert_eq!(total, 2, "{:?}", names(&legal));
    assert!(names(&legal).contains(&"Conveyancing Tool".to_string()));

    // A vertical narrows to it.
    let (conv, _) = svc
        .list_published(list("conveyancing", false))
        .await
        .unwrap();
    assert_eq!(names(&conv), vec!["Conveyancing Tool"]);

    // Cross-industry tools come after the sector's own.
    let (with_cross, total) = svc.list_published(list("legal", true)).await.unwrap();
    assert_eq!(total, 3);
    assert_eq!(
        with_cross.last().map(|a| a.name.as_str()),
        Some("General Tool")
    );

    // Upper-case and "all" behave like the other single-value filters.
    let (upper, _) = svc.list_published(list("LEGAL", false)).await.unwrap();
    assert_eq!(upper.len(), 2);
    let (all, _) = svc.list_published(list("all", false)).await.unwrap();
    assert_eq!(all.len(), 3);

    // Search covers industry and vertical names.
    let (found, _) = svc
        .list_published(ListQuery {
            q: Some("conveyancing".into()),
            limit: 50,
            ..Default::default()
        })
        .await
        .unwrap();
    assert_eq!(names(&found), vec!["Conveyancing Tool"]);

    // Counts are of published entries only: the draft is not counted.
    let tree = svc.industries(Some(ShowcaseKind::App)).await.unwrap();
    let legal_node = tree.iter().find(|n| n.slug == "legal").expect("legal");
    assert_eq!(legal_node.count, 2);
    let conv_node = legal_node
        .verticals
        .iter()
        .find(|v| v.slug == "conveyancing")
        .expect("conveyancing");
    assert_eq!(conv_node.count, 1);

    // The migration replays cleanly on every boot.
    let up = include_str!("../../../migrations/0008_showcase_industries.up.sql");
    pool.execute(up).await.expect("replay 0008");
    let again = svc.industries(None).await.unwrap();
    assert_eq!(again.len(), tree.len());

    drop_db(&admin_url, pool, &db).await;
}
