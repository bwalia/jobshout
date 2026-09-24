//! RSS 2.0 for the whole hub, and an iTunes-tagged podcast feed.

use std::fmt::Write;

use chrono::Utc;
use jobshout_domain::{Insight, InsightKind};

pub struct FeedSite<'a> {
    /// Absolute origin with no trailing slash, e.g. `https://jobshout.com`.
    pub base: &'a str,
}

impl FeedSite<'_> {
    fn item_url(&self, item: &Insight) -> String {
        format!("{}/insights/{}", self.base, item.slug)
    }
}

pub fn escape(s: &str) -> String {
    let mut out = String::with_capacity(s.len());
    for c in s.chars() {
        match c {
            '&' => out.push_str("&amp;"),
            '<' => out.push_str("&lt;"),
            '>' => out.push_str("&gt;"),
            '"' => out.push_str("&quot;"),
            '\'' => out.push_str("&apos;"),
            // Control characters are illegal in XML 1.0 even when escaped.
            c if (c as u32) < 0x20 && !matches!(c, '\t' | '\n' | '\r') => {}
            c => out.push(c),
        }
    }
    out
}

fn kind_label(kind: InsightKind) -> &'static str {
    match kind {
        InsightKind::Post => "Post",
        InsightKind::Article => "Article",
        InsightKind::Blog => "Blog",
        InsightKind::Podcast => "Podcast",
        InsightKind::Video => "Video",
    }
}

pub fn rss(site: &FeedSite, items: &[Insight]) -> String {
    let mut x = String::new();
    let now = Utc::now().to_rfc2822();
    let _ = write!(
        x,
        r#"<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
<title>JobShout Insights</title>
<link>{base}/insights</link>
<atom:link href="{base}/insights/rss.xml" rel="self" type="application/rss+xml"/>
<description>AI trends, job-market shifts, articles, podcasts and videos from JobShout.</description>
<language>en-gb</language>
<lastBuildDate>{now}</lastBuildDate>
"#,
        base = escape(site.base),
    );
    for item in items {
        let url = escape(&site.item_url(item));
        let _ = write!(
            x,
            "<item>\n<title>{title}</title>\n<link>{url}</link>\n<guid isPermaLink=\"true\">{url}</guid>\n<category>{kind}</category>\n",
            title = escape(&item.title),
            kind = kind_label(item.kind),
        );
        for t in &item.topics {
            let _ = writeln!(x, "<category>{}</category>", escape(&t.name));
        }
        if !item.author_display_name.is_empty() {
            let _ = writeln!(
                x,
                "<dc:creator xmlns:dc=\"http://purl.org/dc/elements/1.1/\">{}</dc:creator>",
                escape(&item.author_display_name)
            );
        }
        if let Some(at) = item.published_at {
            let _ = writeln!(x, "<pubDate>{}</pubDate>", at.to_rfc2822());
        }
        let _ = writeln!(x, "<description>{}</description>", escape(&item.summary));
        if !item.body_html.is_empty() {
            let _ = writeln!(
                x,
                "<content:encoded>{}</content:encoded>",
                escape(&item.body_html)
            );
        }
        x.push_str("</item>\n");
    }
    x.push_str("</channel>\n</rss>\n");
    x
}

fn mime_for(url: &str) -> &'static str {
    let lower = url.to_ascii_lowercase();
    let path = lower.split(['?', '#']).next().unwrap_or_default();
    match path.rsplit_once('.').map(|(_, e)| e) {
        Some("m4a") => "audio/x-m4a",
        Some("aac") => "audio/aac",
        Some("ogg" | "oga") => "audio/ogg",
        Some("wav") => "audio/wav",
        Some("mp4" | "m4v") => "video/mp4",
        _ => "audio/mpeg",
    }
}

fn hms(secs: i32) -> String {
    format!(
        "{:02}:{:02}:{:02}",
        secs / 3600,
        (secs % 3600) / 60,
        secs % 60
    )
}

/// Podcast apps need a real file enclosure, so episodes hosted only on
/// Spotify/Apple/YouTube are listed on the site but left out of this feed.
pub fn podcast(site: &FeedSite, cover_url: &str, items: &[Insight]) -> String {
    let mut x = String::new();
    let _ = write!(
        x,
        r#"<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" xmlns:atom="http://www.w3.org/2005/Atom">
<channel>
<title>JobShout Insights Podcast</title>
<link>{base}/insights?kind=podcast</link>
<atom:link href="{base}/insights/podcast.xml" rel="self" type="application/rss+xml"/>
<description>Conversations on AI, work and the job market.</description>
<language>en-gb</language>
<itunes:author>JobShout</itunes:author>
<itunes:image href="{cover}"/>
<itunes:category text="Business"><itunes:category text="Careers"/></itunes:category>
<itunes:category text="Technology"/>
<itunes:explicit>false</itunes:explicit>
<itunes:type>episodic</itunes:type>
"#,
        base = escape(site.base),
        cover = escape(cover_url),
    );
    for item in items
        .iter()
        .filter(|i| i.kind == InsightKind::Podcast && i.embed_provider == "file")
    {
        let url = escape(&site.item_url(item));
        let _ = write!(
            x,
            "<item>\n<title>{title}</title>\n<link>{url}</link>\n<guid isPermaLink=\"false\">{id}</guid>\n<description>{summary}</description>\n<enclosure url=\"{media}\" length=\"0\" type=\"{mime}\"/>\n<itunes:explicit>false</itunes:explicit>\n",
            title = escape(&item.title),
            id = item.id,
            summary = escape(&item.summary),
            media = escape(&item.embed_url),
            mime = mime_for(&item.embed_url),
        );
        if let Some(at) = item.published_at {
            let _ = writeln!(x, "<pubDate>{}</pubDate>", at.to_rfc2822());
        }
        if let Some(d) = item.duration_seconds {
            let _ = writeln!(x, "<itunes:duration>{}</itunes:duration>", hms(d));
        }
        x.push_str("</item>\n");
    }
    x.push_str("</channel>\n</rss>\n");
    x
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::tests::item;
    use jobshout_domain::{InsightStatus, InsightTopic};

    fn published(kind: InsightKind, title: &str) -> Insight {
        let mut i = item(InsightStatus::Published, "a@example.com");
        i.kind = kind;
        i.title = title.into();
        i.slug = "a-b".into();
        i.summary = "Summary with <b>tags</b> & ampersands".into();
        i.body_html = "<p>Hello &amp; <em>world</em></p>".into();
        i.published_at = Some(Utc::now());
        i.topics = vec![InsightTopic {
            slug: "ai-trends".into(),
            name: "AI Trends".into(),
            description: String::new(),
        }];
        i
    }

    #[test]
    fn rss_is_well_formed_and_escaped() {
        let site = FeedSite {
            base: "https://jobshout.com",
        };
        let xml = rss(
            &site,
            &[published(InsightKind::Article, "AI & \"jobs\" <2026>\u{1}")],
        );
        let doc = roxmltree::Document::parse(&xml).expect("well-formed XML");
        let titles: Vec<_> = doc
            .descendants()
            .filter(|n| n.has_tag_name("title"))
            .filter_map(|n| n.text())
            .collect();
        assert!(titles.contains(&"AI & \"jobs\" <2026>"));
        let link = doc.descendants().find(|n| n.has_tag_name("guid")).unwrap();
        assert_eq!(link.text(), Some("https://jobshout.com/insights/a-b"));
    }

    #[test]
    fn podcast_feed_lists_only_file_episodes() {
        let site = FeedSite {
            base: "https://jobshout.com",
        };
        let mut file = published(InsightKind::Podcast, "Episode 1");
        file.embed_provider = "file".into();
        file.embed_url = "https://cdn.example.com/ep1.m4a?x=1&y=2".into();
        file.duration_seconds = Some(3725);
        let mut spotify = published(InsightKind::Podcast, "Episode 2");
        spotify.embed_provider = "spotify".into();
        let article = published(InsightKind::Article, "Not audio");

        let xml = podcast(
            &site,
            "https://jobshout.com/cover.png",
            &[file, spotify, article],
        );
        let doc = roxmltree::Document::parse(&xml).expect("well-formed XML");
        let items: Vec<_> = doc
            .descendants()
            .filter(|n| n.has_tag_name("item"))
            .collect();
        assert_eq!(items.len(), 1);
        let enc = doc
            .descendants()
            .find(|n| n.has_tag_name("enclosure"))
            .unwrap();
        assert_eq!(
            enc.attribute("url"),
            Some("https://cdn.example.com/ep1.m4a?x=1&y=2")
        );
        assert_eq!(enc.attribute("type"), Some("audio/x-m4a"));
        assert!(xml.contains("<itunes:duration>01:02:05</itunes:duration>"));
        assert!(doc.descendants().any(|n| n.tag_name().namespace()
            == Some("http://www.itunes.com/dtds/podcast-1.0.dtd")
            && n.has_tag_name(("http://www.itunes.com/dtds/podcast-1.0.dtd", "image"))));
    }
}
