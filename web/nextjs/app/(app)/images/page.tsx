"use client";

import { useState } from "react";
import { ImageModelPicker, type ImageModelSelection } from "@/components/image/ImageModelPicker";
import { StoredImage } from "@/components/image/StoredImage";
import {
  useGeneratedImages,
  useGenerateImage,
  useImageModels,
} from "@/lib/hooks/useImages";
import type { GenerateImageResponse } from "@/lib/types/image";

/**
 * Shapes offered rather than free-form width and height.
 *
 * Arbitrary dimensions are supported by the API, but three named shapes cover
 * what images are actually used for here — a 16:9 article cover, a 3:2 body
 * illustration, a square avatar — and a pair of number boxes invites sizes that
 * are slower to draw for no benefit.
 */
const SHAPES = [
  { label: "Cover (16:9)", width: 1024, height: 576 },
  { label: "Illustration (3:2)", width: 1024, height: 688 },
  { label: "Square", width: 1024, height: 1024 },
] as const;

/** How a recorded image's source reads. */
const SOURCE_LABELS: Record<string, string> = {
  blog_cover: "Article cover",
  blog_inline: "In article",
  agent_tool: "Agent",
  manual: "Manual",
};

export default function ImagesPage({
  hideHeader = false,
}: {
  hideHeader?: boolean;
}) {
  const { data: modelsInfo } = useImageModels();
  const { data: history, isLoading: historyLoading } = useGeneratedImages(30);
  const generate = useGenerateImage();

  const [prompt, setPrompt] = useState("");
  const [selection, setSelection] = useState<ImageModelSelection>({ provider: "", model: "" });
  const [shape, setShape] = useState<(typeof SHAPES)[number]>(SHAPES[0]);
  const [result, setResult] = useState<GenerateImageResponse | null>(null);

  const enabled = modelsInfo?.enabled ?? true;

  const handleGenerate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!prompt.trim()) return;
    const response = await generate.mutateAsync({
      prompt: prompt.trim(),
      provider: selection.provider || undefined,
      model: selection.model || undefined,
      width: shape.width,
      height: shape.height,
    });
    setResult(response);
  };

  // An image with no URL was generated without object storage configured, so
  // the bytes came back inline. Displaying them from a data URL means the user
  // still sees what they asked for.
  const resultSrc = result
    ? result.url || (result.image_base64 ? `data:image/png;base64,${result.image_base64}` : "")
    : "";

  return (
    <div className="space-y-6">
      {!hideHeader && (
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Images</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Generate images from a description. The same generator draws article covers and
          answers the <code className="text-xs">generate_image</code> tool for agents.
        </p>
      </div>
      )}

      {!enabled && (
        <div className="rounded-xl border border-amber-500/40 bg-amber-500/5 p-4 text-sm">
          <p className="font-medium">Image generation is not configured on this server.</p>
          <p className="mt-1 text-muted-foreground">
            Set <code className="text-xs">GEMINI_API_KEY</code> to use Gemini,{" "}
            <code className="text-xs">IMAGE_BASE_URL</code> to reach the workstation image
            service, or <code className="text-xs">OPENAI_API_KEY</code> for OpenAI.
          </p>
        </div>
      )}

      <section className="rounded-xl border border-border bg-card p-6">
        <form onSubmit={handleGenerate} className="space-y-4">
          <div className="space-y-2">
            <label htmlFor="prompt" className="text-sm font-medium">
              Prompt
            </label>
            <textarea
              id="prompt"
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              rows={3}
              disabled={!enabled}
              placeholder="A flat vector illustration of a satellite dish on a hill at dawn, warm amber tones, minimal editorial style"
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
            />
            <p className="text-xs text-muted-foreground">
              Name a concrete subject and a style. Vague prompts produce vague pictures.
            </p>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <label htmlFor="image-model" className="text-sm font-medium">
                Model
              </label>
              <ImageModelPicker
                id="image-model"
                value={selection}
                onChange={setSelection}
                disabled={!enabled}
              />
            </div>

            <div className="space-y-2">
              <label htmlFor="shape" className="text-sm font-medium">
                Shape
              </label>
              <select
                id="shape"
                value={shape.label}
                onChange={(e) =>
                  setShape(SHAPES.find((s) => s.label === e.target.value) ?? SHAPES[0])
                }
                disabled={!enabled}
                className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm disabled:opacity-50"
              >
                {SHAPES.map((s) => (
                  <option key={s.label} value={s.label}>
                    {s.label} — {s.width}×{s.height}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <button
            type="submit"
            disabled={!enabled || !prompt.trim() || generate.isPending}
            className="inline-flex h-10 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-50"
          >
            {/* The wait is long enough that a static label reads as a hung page. */}
            {generate.isPending ? "Drawing… (this takes 15–30s)" : "Generate"}
          </button>
        </form>
      </section>

      {result && resultSrc && (
        <section className="rounded-xl border border-border bg-card p-6">
          <h2 className="text-base font-semibold">Result</h2>
          <StoredImage
            src={resultSrc}
            alt={prompt}
            className="mt-4 w-full rounded-lg border border-border"
          />
          <dl className="mt-4 grid grid-cols-2 gap-3 text-xs sm:grid-cols-4">
            <div>
              <dt className="text-muted-foreground">Provider</dt>
              <dd className="font-medium">{result.provider}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Model</dt>
              <dd className="font-medium">{result.model}</dd>
            </div>
            <div>
              {/* The seed is shown because it is the only way to reproduce this
                  exact image later. */}
              <dt className="text-muted-foreground">Seed</dt>
              <dd className="font-mono font-medium">{result.seed}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Took</dt>
              <dd className="font-medium">{(result.duration_ms / 1000).toFixed(1)}s</dd>
            </div>
          </dl>
          {!result.url && (
            <p className="mt-3 text-xs text-amber-600 dark:text-amber-400">
              Object storage is not configured, so this image has no permanent URL and will be
              gone when you leave the page.
            </p>
          )}
        </section>
      )}

      <section className="rounded-xl border border-border bg-card p-6">
        <h2 className="text-base font-semibold">Recent</h2>
        {historyLoading ? (
          <p className="mt-4 text-sm text-muted-foreground">Loading…</p>
        ) : !history?.length ? (
          <p className="mt-4 text-sm text-muted-foreground">
            Nothing generated yet.
          </p>
        ) : (
          <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {history.map((img) => (
              <figure key={img.id} className="rounded-lg border border-border p-3">
                {img.url ? (
                  <StoredImage
                    src={img.url}
                    alt={img.prompt}
                    loading="lazy"
                    className="aspect-video w-full rounded object-cover"
                  />
                ) : (
                  <div className="flex aspect-video w-full items-center justify-center rounded bg-muted text-xs text-muted-foreground">
                    not stored
                  </div>
                )}
                <figcaption className="mt-2 space-y-1">
                  <p className="line-clamp-2 text-xs" title={img.prompt}>
                    {img.prompt}
                  </p>
                  <p className="text-[11px] text-muted-foreground">
                    {SOURCE_LABELS[img.source] ?? img.source} · {img.model} · seed {img.seed}
                  </p>
                </figcaption>
              </figure>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
