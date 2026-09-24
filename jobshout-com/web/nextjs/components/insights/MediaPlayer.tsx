import type { Insight } from "@/lib/insights";

const VIDEO_FILE = /\.(mp4|webm|mov|m4v)(\?|#|$)/i;

/**
 * Plays an item's media. `embed_url` is built by the API from an allowlisted
 * provider and a parsed id, so it is safe to put in an iframe as-is.
 */
export function MediaPlayer({ item }: { item: Insight }) {
  const src = item.embed_url;
  if (!src) return null;
  const title = `${item.kind === "podcast" ? "Episode" : "Video"}: ${item.title}`;

  switch (item.embed_provider) {
    case "youtube":
    case "vimeo":
      return (
        <div className="aspect-video overflow-hidden rounded-card border border-line bg-ink shadow-card">
          <iframe
            src={src}
            title={title}
            className="h-full w-full"
            loading="lazy"
            allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; fullscreen"
            referrerPolicy="strict-origin-when-cross-origin"
            allowFullScreen
          />
        </div>
      );
    case "spotify":
    case "apple": {
      const show = /\/show\//.test(src) || (item.embed_provider === "apple" && !src.includes("?i="));
      return (
        <iframe
          src={src}
          title={title}
          className="w-full overflow-hidden rounded-card border border-line"
          style={{ height: show ? 450 : item.embed_provider === "spotify" ? 232 : 175 }}
          loading="lazy"
          allow="autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture"
        />
      );
    }
    case "file":
      return VIDEO_FILE.test(src) ? (
        <video
          controls
          preload="metadata"
          className="aspect-video w-full rounded-card border border-line bg-ink shadow-card"
          aria-label={title}
        >
          <source src={src} />
          Your browser cannot play this video. <a href={src}>Download it</a> instead.
        </video>
      ) : (
        <div className="surface-card p-4 sm:p-5">
          <audio controls preload="metadata" className="w-full" aria-label={title}>
            <source src={src} />
            Your browser cannot play this audio. <a href={src}>Download it</a> instead.
          </audio>
        </div>
      );
    default:
      return null;
  }
}

export function Transcript({ text }: { text: string }) {
  if (!text.trim()) return null;
  return (
    <details className="group surface-card mt-6 p-0">
      <summary className="flex min-h-[48px] cursor-pointer list-none items-center justify-between gap-3 px-5 text-sm font-semibold text-ink">
        Transcript
        <span aria-hidden className="text-mute transition-transform duration-200 group-open:rotate-45">
          +
        </span>
      </summary>
      <div className="whitespace-pre-wrap break-words border-t border-line px-5 py-4 text-sm leading-relaxed text-body">
        {text}
      </div>
    </details>
  );
}
