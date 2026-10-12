//! Members, follows and the feed against a real Postgres.
//!
//! Skipped unless SHOWCASE_TEST_DATABASE_URL points at a server this test
//! may create and drop a scratch database on, e.g.
//! `postgres://postgres@127.0.0.1:5432/postgres`.

use jobshout_content::Actor;
use jobshout_domain::{
    FeedItemKind, MemberInput, ShowcaseAppInput, ShowcaseAppType, ShowcaseBuildMethod,
    ShowcaseKind, ShowcaseLinkInput, ShowcaseMaturity, ShowcasePricing, ShowcaseVisibility,
};
use jobshout_showcase::ShowcaseService;
use jobshout_users::MemberService;
use sqlx::{Connection, Executor, PgConnection, PgPool};

/// Staff, so entries publish without a review round.
fn person(email: &str, name: &str) -> Actor {
    Actor {
        email: email.into(),
        name: name.into(),
        is_staff: true,
        agent: false,
    }
}

fn entry(kind: ShowcaseKind, name: &str, uses: &[&str]) -> ShowcaseAppInput {
    ShowcaseAppInput {
        kind: Some(kind),
        name: name.into(),
        tagline: format!("{name} tagline"),
        description_md: "A test entry for follows. It exists only inside a scratch database that this test creates, fills, checks and then drops again.".into(),
        app_type: Some(ShowcaseAppType::AiApplication),
        maturity: Some(ShowcaseMaturity::Beta),
        build_method: Some(ShowcaseBuildMethod::HumanAi),
        pricing: Some(ShowcasePricing::Free),
        repo_url: "https://example.com/source".into(),
        industries: Some(vec!["cross-industry".into()]),
        agent_links: uses
            .iter()
            .map(|s| ShowcaseLinkInput {
                slug: s.to_string(),
                role: String::new(),
            })
            .collect(),
        ai_models: vec!["claude-opus-5-5".into()],
        capabilities: if kind == ShowcaseKind::App {
            vec![]
        } else {
            vec!["research".into()]
        },
        submit: true,
        ..Default::default()
    }
}

async fn scratch(admin_url: &str) -> (PgPool, String) {
    let name = format!("follows_{}", uuid::Uuid::new_v4().simple());
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

#[tokio::test]
async fn follow_members_and_agents_into_one_feed() {
    let Ok(admin_url) = std::env::var("SHOWCASE_TEST_DATABASE_URL") else {
        eprintln!("SHOWCASE_TEST_DATABASE_URL unset; skipping");
        return;
    };
    let (pool, db) = scratch(&admin_url).await;
    let showcase = ShowcaseService::new(pool.clone());
    let members = MemberService::new(pool.clone());

    let alice = person("Alice@Example.com", "Alice Builder");
    let carol = person("carol@example.com", "Carol");
    let bob = person("bob@example.com", "Bob");

    // Alice already has public work before members exist: backfill finds her.
    let agent = showcase
        .create(&alice, entry(ShowcaseKind::Agent, "Agent X", &[]))
        .await
        .expect("create agent");
    assert_eq!(members.backfill().await.unwrap(), 1);
    assert_eq!(
        members.backfill().await.unwrap(),
        0,
        "backfill is idempotent"
    );
    let (alice_m, _) = members.profile("alice-builder", None).await.unwrap();
    assert_eq!(alice_m.display_name, "Alice Builder");
    assert_eq!(alice_m.entry_count, 1);
    let agent = showcase.get_by_slug(&agent.slug, None).await.unwrap();
    assert_eq!(agent.creator_handle.as_deref(), Some("alice-builder"));
    assert!(agent.creator_email.is_none(), "public view hides the email");

    // Bob follows Alice and her agent; nobody follows themselves.
    let state = members.follow(&bob, "alice-builder", true).await.unwrap();
    assert!(state.following);
    assert_eq!(state.follower_count, 1);
    assert_eq!(
        members
            .follow(&bob, "alice-builder", true)
            .await
            .unwrap()
            .follower_count,
        1,
        "following twice counts once"
    );
    assert!(members.follow(&alice, "alice-builder", true).await.is_err());
    assert_eq!(showcase.follow(&bob, &agent.slug, true).await.unwrap(), 1);
    let seen_by_bob = showcase.get_by_slug(&agent.slug, Some(&bob)).await.unwrap();
    assert!(seen_by_bob.following);
    assert_eq!(seen_by_bob.follower_count, 1);

    // Alice ships an app built with her agent: one "new" story, not two.
    showcase
        .create(&alice, entry(ShowcaseKind::App, "App A", &[&agent.slug]))
        .await
        .expect("create app a");
    // Carol, whom Bob does not follow, uses the agent too.
    members.ensure(&carol).await.unwrap();
    showcase
        .create(&carol, entry(ShowcaseKind::App, "App C", &[&agent.slug]))
        .await
        .expect("create app c");
    // Alice's private work never reaches the feed.
    let mut private = entry(ShowcaseKind::App, "Secret App", &[]);
    private.visibility = Some(ShowcaseVisibility::Private);
    showcase
        .create(&alice, private)
        .await
        .expect("create private");
    // Alice revises her live agent.
    let mut revised = entry(ShowcaseKind::Agent, "Agent X", &[]);
    revised.version = "2.0".into();
    showcase
        .update(&alice, agent.id, revised)
        .await
        .expect("revise agent");

    let feed = members.feed(&bob, None, 50).await.unwrap();
    let story: Vec<(FeedItemKind, String)> = feed
        .iter()
        .map(|i| (i.kind, i.entry.as_ref().unwrap().name.clone()))
        .collect();
    assert!(
        story.contains(&(FeedItemKind::NewEntry, "App A".into())),
        "{story:?}"
    );
    assert!(
        story.contains(&(FeedItemKind::NewEntry, "Agent X".into())),
        "{story:?}"
    );
    assert!(
        story.contains(&(FeedItemKind::UsedIn, "App C".into())),
        "{story:?}"
    );
    assert!(
        story.contains(&(FeedItemKind::EntryUpdated, "Agent X".into())),
        "{story:?}"
    );
    assert!(!story.iter().any(|(_, n)| n == "Secret App"), "{story:?}");
    assert!(
        !story.contains(&(FeedItemKind::UsedIn, "App A".into())),
        "a followed member's new app is not repeated as used_in: {story:?}"
    );
    let used = feed
        .iter()
        .find(|i| i.kind == FeedItemKind::UsedIn)
        .unwrap();
    assert_eq!(used.uses.as_ref().unwrap().slug, agent.slug);
    let new_app = feed
        .iter()
        .find(|i| i.kind == FeedItemKind::NewEntry)
        .unwrap();
    assert_eq!(new_app.actor.as_ref().unwrap().handle, "alice-builder");
    assert!(feed.windows(2).all(|w| w[0].at >= w[1].at), "newest first");

    // Paging back from the last item returns only older ones.
    let last = feed.last().unwrap().at;
    let older = members.feed(&bob, Some(last), 50).await.unwrap();
    assert!(older.iter().all(|i| i.at < last));

    // What Bob follows.
    let (ms, es) = members.following(&bob).await.unwrap();
    assert_eq!(ms.len(), 1);
    assert_eq!(ms[0].handle, "alice-builder");
    assert_eq!(es.len(), 1);
    assert_eq!(es[0].entry.slug, agent.slug);

    // The directory lists people with public work, most followed first.
    let (dir, total) = members.directory(None, Some(&bob), 10, 0).await.unwrap();
    assert_eq!(total, 2, "{dir:?}");
    assert_eq!(dir[0].handle, "alice-builder");
    assert!(dir[0].following);
    let (found, _) = members
        .directory(Some("caro".into()), None, 10, 0)
        .await
        .unwrap();
    assert_eq!(found.len(), 1);
    let (none, _) = members
        .directory(Some("%".into()), None, 10, 0)
        .await
        .unwrap();
    assert!(none.is_empty(), "% matches literally");

    // Headlines.
    let me = members
        .set_profile(
            &alice,
            MemberInput {
                headline: "Builds agents".into(),
            },
        )
        .await
        .unwrap();
    assert_eq!(me.headline, "Builds agents");
    assert_eq!(me.follower_count, 1);

    // Unfollowing.
    let state = members.follow(&bob, "alice-builder", false).await.unwrap();
    assert_eq!(state.follower_count, 0);
    assert_eq!(showcase.follow(&bob, &agent.slug, false).await.unwrap(), 0);
    assert!(members.feed(&bob, None, 50).await.unwrap().is_empty());

    // The migration replays cleanly.
    let up = include_str!("../../../migrations/0009_follows.up.sql");
    pool.execute(up).await.expect("replay 0009");

    drop_db(&admin_url, pool, &db).await;
}
