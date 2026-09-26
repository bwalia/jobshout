"use client";

import Link from "next/link";
import { useEffect, useState, useTransition } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { EMPTY_FORM_STATE } from "@/lib/form-state";
import { previewAction, saveInsightAction } from "@/app/insights/actions";
import { CheckCircleIcon } from "@/components/icons";
import { KindIcon } from "@/components/insights/KindIcon";
import { Button, ErrorNotice, Field, Input, Textarea, buttonClass, cx } from "@/components/ui";
import {
  INSIGHT_KINDS,
  KIND_META,
  type Insight,
  type InsightKind,
  type InsightTopic,
} from "@/lib/insights";

const MAX_TOPICS = 3;

function SubmitButtons({ isStaff, kindLabel }: { isStaff: boolean; kindLabel: string }) {
  const { pending } = useFormStatus();
  return (
    <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
      <Button type="submit" name="intent" value="draft" variant="secondary" size="lg" disabled={pending}>
        Save draft
      </Button>
      <Button type="submit" name="intent" value="submit" size="lg" disabled={pending}>
        {pending ? "Saving…" : isStaff ? `Publish ${kindLabel.toLowerCase()}` : "Submit for review"}
      </Button>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset className="surface-card space-y-6 p-5 sm:p-7">
      <legend className="float-left mb-3 w-full font-display text-lg font-semibold text-ink">
        {title}
      </legend>
      <div className="clear-both space-y-6">{children}</div>
    </fieldset>
  );
}

export function InsightForm({
  topics,
  isStaff,
  initial,
  defaultKind,
}: {
  topics: InsightTopic[];
  isStaff: boolean;
  initial?: Insight | null;
  defaultKind?: InsightKind;
}) {
  const [state, action] = useFormState(saveInsightAction, EMPTY_FORM_STATE);
  const [kind, setKind] = useState<InsightKind>(initial?.kind ?? defaultKind ?? "article");
  const [body, setBody] = useState(initial?.body_md ?? "");
  const [chosen, setChosen] = useState<string[]>(initial?.topics.map((t) => t.slug) ?? []);
  const [tab, setTab] = useState<"write" | "preview">("write");
  const [preview, setPreview] = useState("");
  const [previewing, startPreview] = useTransition();

  // Rendered by the API so the preview is exactly what will be published.
  useEffect(() => {
    if (tab !== "preview") return;
    const t = setTimeout(() => {
      startPreview(async () => {
        try {
          setPreview(await previewAction(body));
        } catch {
          setPreview("<p><em>Preview is unavailable right now.</em></p>");
        }
      });
    }, 150);
    return () => clearTimeout(t);
  }, [tab, body]);

  const words = body.trim() ? body.trim().split(/\s+/).length : 0;
  const media = kind === "podcast" || kind === "video";
  const errors = state.fieldErrors ?? {};
  const bodyHint: Record<InsightKind, string> = {
    post: `Up to 500 words. ${words}/500`,
    article: `Markdown. At least 80 words to submit — ${words} so far.`,
    blog: `Markdown. At least 80 words to submit — ${words} so far.`,
    podcast: "Show notes in Markdown: what the episode covers, guests, links mentioned.",
    video: "Description in Markdown: what viewers will learn, chapters, links.",
  };

  if (state.ok && state.result?.id) {
    const status = state.result.status;
    return (
      <div className="surface-card animate-rise-in p-8 text-center sm:p-12">
        <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-good/10 text-good">
          <CheckCircleIcon className="h-7 w-7" />
        </span>
        <h2 className="mt-5 font-display text-2xl font-semibold text-ink">
          {status === "published" ? "It is live" : status === "pending_review" ? "Submitted for review" : "Draft saved"}
        </h2>
        <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-mute">
          {status === "published"
            ? "It is on the Insights front page and in the feeds now."
            : status === "pending_review"
              ? "An editor will read it before it goes live. You can track it, and any requested changes, under Your insights."
              : "Only you can see it. Come back and submit it when it is ready."}
        </p>
        <div className="mt-7 flex flex-wrap justify-center gap-3">
          <Link href={`/insights/${state.result.id}`} className={buttonClass("primary", "md")}>
            {status === "published" ? "View it" : "Preview it"}
          </Link>
          <Link href="/insights/mine" className={buttonClass("secondary", "md")}>
            Your insights
          </Link>
        </div>
      </div>
    );
  }

  return (
    <form action={action} className="space-y-8" noValidate>
      {initial ? <input type="hidden" name="id" value={initial.id} /> : null}
      {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}

      <Section title="What are you publishing?">
        <div role="radiogroup" aria-label="Format" className="grid grid-cols-2 gap-2.5 sm:grid-cols-5">
          {INSIGHT_KINDS.map((k) => {
            const active = kind === k;
            return (
              <label
                key={k}
                className={cx(
                  "relative flex min-h-[88px] cursor-pointer flex-col items-start gap-2 rounded-xl border p-3.5 transition-colors duration-200",
                  "has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-shout",
                  active ? "border-shout bg-shout/[0.07]" : "border-line bg-surface hover:border-edge",
                )}
              >
                <input
                  type="radio"
                  name="kind"
                  value={k}
                  checked={active}
                  onChange={() => setKind(k)}
                  className="sr-only"
                />
                <KindIcon kind={k} className={cx("h-5 w-5", active ? "text-shout" : "text-mute")} />
                <span className="text-sm font-semibold text-ink">{KIND_META[k].label}</span>
              </label>
            );
          })}
        </div>
        <p className="text-sm text-mute">{KIND_META[kind].blurb}</p>
      </Section>

      <Section title="The story">
        <Field label="Headline" htmlFor="title" required error={errors.title}>
          <Input
            id="title"
            name="title"
            defaultValue={initial?.title}
            maxLength={160}
            placeholder="What changed, and why it matters"
            aria-invalid={Boolean(errors.title)}
          />
        </Field>
        <Field
          label="Summary"
          htmlFor="summary"
          hint="One or two sentences for cards, search results and social previews."
          error={errors.summary}
        >
          <Textarea id="summary" name="summary" rows={2} maxLength={300} defaultValue={initial?.summary} />
        </Field>

        {media ? (
          <div className="grid gap-6 sm:grid-cols-[1fr_10rem]">
            <Field
              label={kind === "podcast" ? "Episode link" : "Video link"}
              htmlFor="media_url"
              required
              hint={
                kind === "podcast"
                  ? "Spotify, Apple Podcasts or YouTube link, or a direct .mp3/.m4a file (needed for the podcast feed)."
                  : "YouTube or Vimeo link, or a direct .mp4/.webm file."
              }
              error={errors.media_url}
            >
              <Input
                id="media_url"
                name="media_url"
                type="url"
                inputMode="url"
                defaultValue={initial?.media_url}
                placeholder="https://"
                aria-invalid={Boolean(errors.media_url)}
              />
            </Field>
            <Field label="Length (minutes)" htmlFor="duration_minutes" error={errors.duration}>
              <Input
                id="duration_minutes"
                name="duration_minutes"
                type="number"
                min={0}
                max={1440}
                step="0.5"
                defaultValue={initial?.duration_seconds ? Math.round(initial.duration_seconds / 60) : ""}
              />
            </Field>
          </div>
        ) : null}

        <div>
          <div className="flex items-end justify-between gap-3">
            <label htmlFor="body_md" className="block text-sm font-semibold text-ink">
              {media ? (kind === "podcast" ? "Show notes" : "Description") : "Body"}
              {kind === "article" || kind === "blog" ? (
                <span className="ml-1 text-shout" aria-hidden>*</span>
              ) : (
                <span className="ml-2 text-xs font-normal text-mute">optional</span>
              )}
            </label>
            <div role="tablist" aria-label="Editor mode" className="flex rounded-pill border border-line bg-raised p-0.5">
              {(["write", "preview"] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  role="tab"
                  aria-selected={tab === t}
                  aria-controls={`editor-${t}`}
                  onClick={() => setTab(t)}
                  className={cx(
                    "min-h-[32px] rounded-pill px-3.5 text-xs font-semibold capitalize transition-colors duration-200",
                    tab === t ? "bg-surface text-ink shadow-card" : "text-mute hover:text-ink",
                  )}
                >
                  {t}
                </button>
              ))}
            </div>
          </div>
          <p id="body_md-hint" className="mt-1 text-xs leading-relaxed text-mute">
            {bodyHint[kind]}
          </p>
          <div className="mt-2">
            <div id="editor-write" role="tabpanel" hidden={tab !== "write"}>
              <Textarea
                id="body_md"
                name="body_md"
                rows={kind === "post" ? 6 : 16}
                value={body}
                onChange={(e) => setBody(e.target.value)}
                aria-describedby="body_md-hint"
                aria-invalid={Boolean(errors.body_md)}
                className="font-mono text-[0.9rem]"
                placeholder={"## Why this matters\n\nStart with the change, then the evidence, then what readers should do about it."}
              />
            </div>
            <div
              id="editor-preview"
              role="tabpanel"
              hidden={tab !== "preview"}
              aria-busy={previewing}
              className="min-h-[12rem] rounded-xl border border-line bg-surface px-5 py-4"
            >
              {preview ? (
                <div className="insight-prose" dangerouslySetInnerHTML={{ __html: preview }} />
              ) : (
                <p className="text-sm text-mute">{previewing ? "Rendering…" : "Nothing to preview yet."}</p>
              )}
            </div>
          </div>
          {errors.body_md ? (
            <p role="alert" className="mt-1.5 text-xs font-medium text-shout">
              {errors.body_md}
            </p>
          ) : null}
        </div>

        {media ? (
          <Field
            label="Transcript"
            htmlFor="transcript"
            hint="Makes the episode searchable and accessible to people who cannot listen."
          >
            <Textarea id="transcript" name="transcript" rows={5} defaultValue={initial?.transcript} />
          </Field>
        ) : (
          <Field
            label={kind === "post" ? "Link" : "Source link"}
            htmlFor="link_url"
            hint={kind === "post" ? "The story or announcement you are sharing." : "Primary source, if you are responding to one."}
            error={errors.link_url}
          >
            <Input id="link_url" name="link_url" type="url" inputMode="url" defaultValue={initial?.link_url} placeholder="https://" />
          </Field>
        )}
      </Section>

      <Section title="Topics and cover">
        <div>
          <p className="text-sm font-semibold text-ink" id="topics-label">
            Topics <span className="ml-1 text-xs font-normal text-mute">up to {MAX_TOPICS}</span>
          </p>
          <div role="group" aria-labelledby="topics-label" className="mt-3 flex flex-wrap gap-2">
            {topics.map((t) => {
              const on = chosen.includes(t.slug);
              const full = !on && chosen.length >= MAX_TOPICS;
              return (
                <label
                  key={t.slug}
                  className={cx(
                    "inline-flex min-h-[40px] cursor-pointer items-center gap-2 rounded-pill border px-3.5 text-sm transition-colors duration-200",
                    "has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-shout",
                    on ? "border-shout/50 bg-shout/10 font-semibold text-ink" : "border-line text-body hover:border-edge",
                    full && "cursor-not-allowed opacity-50",
                  )}
                >
                  <input
                    type="checkbox"
                    name="topics"
                    value={t.slug}
                    checked={on}
                    disabled={full}
                    onChange={() =>
                      setChosen((c) => (on ? c.filter((s) => s !== t.slug) : [...c, t.slug]))
                    }
                    className="sr-only"
                  />
                  {t.name}
                </label>
              );
            })}
          </div>
          {errors.topics ? (
            <p role="alert" className="mt-1.5 text-xs font-medium text-shout">
              {errors.topics}
            </p>
          ) : null}
        </div>

        <div className="grid gap-6 sm:grid-cols-2">
          <Field
            label="Cover image URL"
            htmlFor="cover_image_url"
            hint="Landscape works best (16:9). Leave empty for a branded tile."
            error={errors.cover_image_url}
          >
            <Input id="cover_image_url" name="cover_image_url" type="url" inputMode="url" defaultValue={initial?.cover_image_url} placeholder="https://" />
          </Field>
          <Field
            label="Cover description"
            htmlFor="cover_image_alt"
            hint="Required with a cover: what the image shows, for screen readers."
          >
            <Input id="cover_image_alt" name="cover_image_alt" defaultValue={initial?.cover_image_alt} />
          </Field>
        </div>
      </Section>

      {!isStaff ? (
        <p className="text-sm leading-relaxed text-mute">
          Community submissions are read by an editor before they go live. Keep claims sourced, and do not
          paste anything you do not have the right to publish.
        </p>
      ) : null}

      <SubmitButtons isStaff={isStaff} kindLabel={KIND_META[kind].label} />
    </form>
  );
}
