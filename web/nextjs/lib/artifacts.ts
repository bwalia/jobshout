import { resolveStoredImageSrc } from "@/lib/api/images";
import type { ArtifactItem } from "@/lib/types/artifact";
import type { BlogRun } from "@/lib/types/blog";
import type { GeneratedImage } from "@/lib/types/image";

const IMAGE_SOURCE_LABELS: Record<string, string> = {
  blog_cover: "Article cover",
  blog_inline: "In article",
  agent_tool: "Agent",
  manual: "Manual",
};

function currentStepLabel(run: BlogRun): string | null {
  return run.steps.find((s) => s.status === "running")?.label ?? null;
}

/**
 * One card per written article. A run that has not produced a body yet still
 * appears, so in-flight and failed work is not hidden from the library.
 */
export function artifactsFromBlogRuns(runs: BlogRun[]): ArtifactItem[] {
  const items: ArtifactItem[] = [];
  for (const run of runs) {
    if (run.articles.length > 0) {
      for (const article of run.articles) {
        items.push({
          id: `article:${article.id}`,
          kind: "article",
          title: article.title || article.topic,
          subtitle:
            article.title && article.topic && article.title !== article.topic
              ? article.topic
              : undefined,
          href: `/articles/${run.id}?article=${article.id}`,
          createdAt: run.created_at,
          status: run.status,
          meta: article.word_count
            ? `${article.word_count.toLocaleString()} words`
            : undefined,
        });
      }
      continue;
    }
    const topic = run.topics[0] ?? "Article run";
    const step = currentStepLabel(run);
    items.push({
      id: `article-run:${run.id}`,
      kind: "article",
      title: run.topics.length > 1 ? `${run.topics.length} articles` : topic,
      subtitle: run.topics.length > 1 ? run.topics.join(" · ") : undefined,
      href: `/articles/${run.id}`,
      createdAt: run.created_at,
      status: run.status,
      meta: step ?? undefined,
    });
  }
  return items;
}

/** "Cover of <title>" when the run wrote one article, else a generic label. */
function articleImageTitle(image: GeneratedImage, run?: BlogRun): string {
  const article = run?.articles.length === 1 ? run.articles[0] : undefined;
  const name = article ? article.title || article.topic : "";
  if (!name) return "Article image";
  return image.source === "blog_cover" ? `Cover of ${name}` : `Image in ${name}`;
}

/**
 * An image drawn for an article opens that article; any other image opens the
 * image itself. Pass the loaded runs to name the article on the card.
 */
export function artifactsFromImages(
  images: GeneratedImage[],
  runs: BlogRun[] = []
): ArtifactItem[] {
  const runsById = new Map(runs.map((run) => [run.id, run]));
  return images.map((image) => {
    const runId = image.blog_run_id;
    let href = image.url ? resolveStoredImageSrc(image.url) : undefined;
    if (runId) href = `/articles/${runId}`;
    return {
      id: `image:${image.id}`,
      kind: "image",
      title: runId ? articleImageTitle(image, runsById.get(runId)) : image.prompt,
      subtitle: runId ? image.prompt : undefined,
      href,
      createdAt: image.created_at,
      imageUrl: image.url,
      meta: [IMAGE_SOURCE_LABELS[image.source] ?? image.source, image.model]
        .filter(Boolean)
        .join(" · "),
    };
  });
}

export function sortArtifacts(items: ArtifactItem[]): ArtifactItem[] {
  return [...items].sort((a, b) => {
    const ta = Date.parse(a.createdAt);
    const tb = Date.parse(b.createdAt);
    return (Number.isNaN(tb) ? 0 : tb) - (Number.isNaN(ta) ? 0 : ta);
  });
}
