//! Sample items for dev and int, so the hub is not empty on first boot.
//! Every one says it is a sample, and none quotes a statistic or a person.

use jobshout_domain::{InsightInput, InsightKind};

const NOTE: &str = "_This is sample content seeded for development. Replace it with real reporting before launch._";

fn item(
    kind: InsightKind,
    title: &str,
    summary: &str,
    body: &str,
    topics: &[&str],
) -> InsightInput {
    InsightInput {
        kind: Some(kind),
        title: title.into(),
        summary: summary.into(),
        body_md: format!("{body}\n\n{NOTE}"),
        topics: topics.iter().map(|t| t.to_string()).collect(),
        submit: true,
        ..Default::default()
    }
}

/// (input, featured)
pub fn samples() -> Vec<(InsightInput, bool)> {
    let article = item(
        InsightKind::Article,
        "Sample article: how to read an AI-era job description",
        "Sample content. A walkthrough of what the new wording in AI-related job ads usually means, and which parts to ask about.",
        "## Why job ads changed\n\nMany roles now mention AI tools, even when the day-to-day work has not changed much. That makes it harder to tell whether a team expects you to build with models, use them, or simply be comfortable around them.\n\n## Three questions to ask\n\n1. **Which tools does the team use today?** A named tool is a better signal than a general mention of AI.\n2. **Who owns the model work?** Knowing whether there is a platform team tells you how much you would build yourself.\n3. **How is output reviewed?** Teams with a clear review step usually have thought about quality and risk.\n\n## What to put in your application\n\nDescribe one concrete thing you did with an AI tool, what you checked, and what you changed as a result. Specific beats broad every time.",
        &["skills-careers", "ai-trends"],
    );

    let blog = item(
        InsightKind::Blog,
        "Sample blog: notes from a week of pairing with an AI teammate",
        "Sample content. A first-person style placeholder about working alongside an AI agent for a week.",
        "I spent a week handing the first draft of routine tasks to an AI teammate and keeping the review for myself. This placeholder post stands in for that kind of first-person account.\n\n## What worked\n\nSmall, well-described tasks came back quickly and were easy to check. Writing the task down clearly turned out to be most of the work.\n\n## What did not\n\nAnything that needed context from a hallway conversation went wrong, because the agent never had that context. The fix was writing more down, which helped the humans on the team as well.\n\n## Would I do it again\n\nYes, with a short checklist for reviewing what comes back, and a rule that nothing ships without a person reading it first.",
        &["ai-trends", "hiring"],
    );

    let mut post = item(
        InsightKind::Post,
        "Sample post: new Insights section is open for contributions",
        "Sample content. Signed-in members can now submit posts, articles, podcasts and videos for review.",
        "Insights is where we collect news and analysis about AI and the job market. Sign in, write something useful, and an editor will review it before it goes live.",
        &["job-market"],
    );
    post.link_url = "https://jobshout.com/insights".into();

    let mut podcast = item(
        InsightKind::Podcast,
        "Sample podcast: episode zero",
        "Sample content. A placeholder episode that exercises the audio player and the podcast feed.",
        "Show notes go here: what the episode covers, who is on it, and links mentioned in the conversation.",
        &["ai-trends", "tools-research"],
    );
    podcast.media_url =
        "https://interactive-examples.mdn.mozilla.net/media/cc0-audio/t-rex-roar.mp3".into();
    podcast.duration_seconds = Some(3);
    podcast.transcript = "[Sample transcript] A short sound effect used to test the player.".into();

    let mut video = item(
        InsightKind::Video,
        "Sample video: testing the Insights video player",
        "Sample content. A short public-domain clip that exercises the video layout.",
        "Video descriptions go here: what the viewer will learn, chapters, and links.",
        &["tools-research"],
    );
    video.media_url =
        "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4".into();
    video.duration_seconds = Some(5);
    video.transcript = "[Sample transcript] No speech — a flower opening in time-lapse.".into();

    let policy = item(
        InsightKind::Article,
        "Sample article: what to watch in workplace AI policy",
        "Sample content. A placeholder explainer on the kinds of policy questions employers are working through.",
        "## The questions employers are asking\n\nMost workplace AI policies try to answer the same few questions: which tools staff may use, what data can go into them, who reviews the output, and how candidates are told when AI is part of hiring.\n\n## Why it matters to job seekers\n\nA clear policy is a good sign. It suggests the employer has thought about fairness in screening and about how your work will be credited.\n\n## What to look for\n\nAsk whether AI is used to screen applications, whether a person makes the final decision, and how you can ask for a human review. Those answers tell you a lot about the culture you would be joining.",
        &["policy-regulation", "hiring"],
    );

    vec![
        (article, true),
        (blog, false),
        (post, false),
        (podcast, false),
        (video, false),
        (policy, false),
    ]
}
