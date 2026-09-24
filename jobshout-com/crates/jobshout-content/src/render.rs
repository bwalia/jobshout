//! Markdown → sanitised HTML. The only HTML the web ever renders for a body.

use pulldown_cmark::{html, Options, Parser};

pub fn markdown_to_html(md: &str) -> String {
    let mut opts = Options::empty();
    opts.insert(Options::ENABLE_TABLES);
    opts.insert(Options::ENABLE_STRIKETHROUGH);
    opts.insert(Options::ENABLE_TASKLISTS);
    let mut raw = String::with_capacity(md.len() * 3 / 2);
    html::push_html(&mut raw, Parser::new_ext(md, opts));

    ammonia::Builder::default()
        .link_rel(Some("noopener noreferrer nofollow ugc"))
        .url_schemes(["http", "https", "mailto"].into_iter().collect())
        .clean(&raw)
        .to_string()
}

pub fn word_count(md: &str) -> usize {
    md.split_whitespace().count()
}

/// Minutes at ~220 words a minute, never zero.
pub fn reading_minutes(md: &str) -> i32 {
    let words = word_count(md) as i32;
    ((words + 219) / 220).max(1)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn renders_markdown() {
        let html = markdown_to_html("## Hello\n\n**bold** and [link](https://example.com)");
        assert!(html.contains("<h2>Hello</h2>"));
        assert!(html.contains("<strong>bold</strong>"));
        assert!(html.contains("rel=\"noopener noreferrer nofollow ugc\""));
    }

    #[test]
    fn strips_scripts_handlers_and_js_links() {
        let html = markdown_to_html(
            "<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n[x](javascript:alert(1))\n\n<iframe src=\"https://evil.example\"></iframe>",
        );
        assert!(!html.contains("<script"));
        assert!(!html.contains("onerror"));
        assert!(!html.contains("javascript:"));
        assert!(!html.contains("<iframe"));
    }

    #[test]
    fn reading_time() {
        assert_eq!(reading_minutes(""), 1);
        assert_eq!(reading_minutes(&"word ".repeat(220)), 1);
        assert_eq!(reading_minutes(&"word ".repeat(221)), 2);
    }
}
