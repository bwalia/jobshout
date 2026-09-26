//! Media URLs → something the page can play.
//!
//! Iframes are only ever built for the hosts below, from an id we parsed out
//! of the URL: the stored `embed_url` is ours, never the submitter's string.
//! Direct audio/video files may live anywhere on https because they play in a
//! plain `<audio>`/`<video>` element, not a frame.

use url::Url;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MediaFormat {
    Audio,
    Video,
    /// A provider player that may carry either (YouTube can host a podcast).
    Embed,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Media {
    pub provider: &'static str,
    pub embed_url: String,
    pub format: MediaFormat,
}

const AUDIO_EXT: &[&str] = &["mp3", "m4a", "aac", "ogg", "oga", "wav"];
const VIDEO_EXT: &[&str] = &["mp4", "webm", "mov", "m4v"];

pub fn resolve(raw: &str) -> Result<Media, String> {
    let url = Url::parse(raw.trim()).map_err(|_| "media URL is not a valid URL".to_string())?;
    if url.scheme() != "https" {
        return Err("media URL must use https".into());
    }
    let host = url.host_str().unwrap_or_default().to_ascii_lowercase();
    let segments: Vec<&str> = url
        .path_segments()
        .map(|s| s.filter(|p| !p.is_empty()).collect())
        .unwrap_or_default();

    match host.as_str() {
        "youtube.com"
        | "www.youtube.com"
        | "m.youtube.com"
        | "youtube-nocookie.com"
        | "www.youtube-nocookie.com" => {
            let id = match segments.as_slice() {
                ["watch"] => url
                    .query_pairs()
                    .find(|(k, _)| k == "v")
                    .map(|(_, v)| v.into_owned()),
                ["embed" | "shorts" | "live", id, ..] => Some((*id).to_string()),
                _ => None,
            };
            youtube(id)
        }
        "youtu.be" => youtube(segments.first().map(|s| (*s).to_string())),
        "vimeo.com" | "www.vimeo.com" | "player.vimeo.com" => {
            let id = segments
                .iter()
                .find(|s| !s.is_empty() && s.chars().all(|c| c.is_ascii_digit()))
                .ok_or("that Vimeo link has no video id")?;
            Ok(Media {
                provider: "vimeo",
                embed_url: format!("https://player.vimeo.com/video/{id}"),
                format: MediaFormat::Video,
            })
        }
        "open.spotify.com" => {
            // Localised links carry an "intl-xx" segment first.
            let rest: Vec<&str> = segments
                .iter()
                .copied()
                .skip_while(|s| s.starts_with("intl-") || *s == "embed")
                .collect();
            match rest.as_slice() {
                [kind @ ("episode" | "show"), id, ..] if is_token(id) => Ok(Media {
                    provider: "spotify",
                    embed_url: format!("https://open.spotify.com/embed/{kind}/{id}"),
                    format: MediaFormat::Audio,
                }),
                _ => Err("link a Spotify episode or show".into()),
            }
        }
        "podcasts.apple.com" | "embed.podcasts.apple.com" => {
            if !segments.iter().any(|s| s.starts_with("id") && s.len() > 2) {
                return Err("link an Apple Podcasts show or episode".into());
            }
            let mut embed = Url::parse("https://embed.podcasts.apple.com").expect("static url");
            embed.set_path(url.path());
            // Keep only the episode selector; drop tracking parameters.
            if let Some((_, i)) = url.query_pairs().find(|(k, _)| k == "i") {
                if i.chars().all(|c| c.is_ascii_digit()) {
                    embed.set_query(Some(&format!("i={i}")));
                }
            }
            Ok(Media {
                provider: "apple",
                embed_url: embed.to_string(),
                format: MediaFormat::Audio,
            })
        }
        _ => {
            let ext = segments
                .last()
                .and_then(|s| s.rsplit_once('.'))
                .map(|(_, e)| e.to_ascii_lowercase())
                .unwrap_or_default();
            let format = if AUDIO_EXT.contains(&ext.as_str()) {
                MediaFormat::Audio
            } else if VIDEO_EXT.contains(&ext.as_str()) {
                MediaFormat::Video
            } else {
                return Err(
                    "use a YouTube, Vimeo, Spotify or Apple Podcasts link, or a direct .mp3/.mp4 file"
                        .into(),
                );
            };
            Ok(Media {
                provider: "file",
                embed_url: url.to_string(),
                format,
            })
        }
    }
}

fn youtube(id: Option<String>) -> Result<Media, String> {
    let id = id.ok_or("that YouTube link has no video id")?;
    if id.len() != 11 || !is_token(&id) {
        return Err("that YouTube link has no valid video id".into());
    }
    Ok(Media {
        provider: "youtube",
        embed_url: format!("https://www.youtube-nocookie.com/embed/{id}"),
        format: MediaFormat::Embed,
    })
}

fn is_token(s: &str) -> bool {
    !s.is_empty()
        && s.chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_')
}

/// An ordinary outbound link (cover image, source link): http(s) only.
pub fn is_web_url(raw: &str) -> bool {
    Url::parse(raw.trim())
        .map(|u| matches!(u.scheme(), "http" | "https") && u.host_str().is_some())
        .unwrap_or(false)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn youtube_variants_normalise_to_nocookie_embed() {
        for raw in [
            "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10s",
            "https://youtu.be/dQw4w9WgXcQ",
            "https://youtube.com/shorts/dQw4w9WgXcQ",
            "https://www.youtube.com/embed/dQw4w9WgXcQ",
        ] {
            let m = resolve(raw).unwrap();
            assert_eq!(m.provider, "youtube");
            assert_eq!(
                m.embed_url,
                "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ"
            );
        }
    }

    #[test]
    fn lookalike_hosts_are_rejected() {
        for raw in [
            "https://youtube.com.evil.example/watch?v=dQw4w9WgXcQ",
            "https://evilyoutube.com/watch?v=dQw4w9WgXcQ",
            "https://open.spotify.com.evil.example/episode/abc",
            "https://example.com/page.html",
        ] {
            assert!(resolve(raw).is_err(), "{raw} should be rejected");
        }
    }

    #[test]
    fn injected_ids_are_rejected() {
        assert!(resolve("https://www.youtube.com/watch?v=abc\"><script>").is_err());
        assert!(resolve("https://open.spotify.com/episode/a%22b").is_err());
    }

    #[test]
    fn http_media_is_rejected() {
        assert!(resolve("http://youtu.be/dQw4w9WgXcQ").is_err());
        assert!(resolve("javascript:alert(1)").is_err());
    }

    #[test]
    fn spotify_apple_vimeo_and_files() {
        let s = resolve("https://open.spotify.com/intl-de/episode/4rOoJ6Egrf8K2IrywzwOMk?si=x")
            .unwrap();
        assert_eq!(
            s.embed_url,
            "https://open.spotify.com/embed/episode/4rOoJ6Egrf8K2IrywzwOMk"
        );

        let a =
            resolve("https://podcasts.apple.com/gb/podcast/show/id123456?i=1000654&utm=x").unwrap();
        assert_eq!(
            a.embed_url,
            "https://embed.podcasts.apple.com/gb/podcast/show/id123456?i=1000654"
        );

        let v = resolve("https://vimeo.com/76979871").unwrap();
        assert_eq!(v.embed_url, "https://player.vimeo.com/video/76979871");

        assert_eq!(
            resolve("https://cdn.example.com/ep1.MP3").unwrap().format,
            MediaFormat::Audio
        );
        assert_eq!(
            resolve("https://cdn.example.com/talk.webm").unwrap().format,
            MediaFormat::Video
        );
    }

    #[test]
    fn web_urls() {
        assert!(is_web_url("https://example.com/a.png"));
        assert!(!is_web_url("javascript:alert(1)"));
        assert!(!is_web_url("data:image/png;base64,AAAA"));
    }
}
