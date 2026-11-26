"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import Link from "next/link";
import { useParams, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import {
  AlertTriangle,
  BarChart3,
  CalendarClock,
  CheckCircle2,
  ExternalLink,
  ImageIcon,
  Link2,
  Loader2,
  MessageCircle,
  RefreshCw,
  Save,
  Send,
  Trash2,
} from "lucide-react";
import {
  useApproveFacebookPost,
  useConnectFacebook,
  useDeleteFacebookPost,
  useDisconnectFacebook,
  useEditPublishedFacebookPost,
  useFacebookAnalytics,
  useFacebookComments,
  useFacebookPosts,
  useFacebookStatus,
  useGenerateFacebookImage,
  usePublishFacebookPost,
  useRedraftFacebookPost,
  useRefreshFacebookPage,
  useReplyToFacebookComment,
  useSaveFacebookProfile,
  useSetFacebookAutomation,
  useUnscheduleFacebookPost,
  useUpdateFacebookPageInfo,
  useUpdateFacebookPost,
} from "@/lib/hooks/useFacebook";
import { cn } from "@/lib/utils/cn";
import { newTaskHref } from "@/lib/agents/links";
import {
  FACEBOOK_MESSAGE_MAX,
  FACEBOOK_STATUS_LABEL,
  type FacebookPage,
  type FacebookPost,
  type FacebookPostStatus,
  type FacebookStatus,
} from "@/lib/types/facebook";

const STATUS_CLASS: Record<FacebookPostStatus, string> = {
  drafting: "bg-signal-live/10 text-signal-live",
  draft: "bg-signal/10 text-signal",
  scheduled: "bg-primary/10 text-primary",
  publishing: "bg-signal-live/10 text-signal-live",
  published: "bg-status-done/10 text-status-done",
  failed: "bg-signal-error/10 text-signal-error",
};

const buttonBase =
  "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-opacity disabled:cursor-not-allowed disabled:opacity-50";
const buttonPrimary = cn(buttonBase, "bg-primary text-primary-foreground hover:opacity-90");
const buttonSecondary = cn(
  buttonBase,
  "border border-border bg-card text-foreground transition-colors hover:bg-muted"
);
const inputClass =
  "w-full rounded-md border border-border bg-card p-2 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary";

function Spin({ on, children }: { on: boolean; children: ReactNode }) {
  return on ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <>{children}</>;
}

function StatusBadge({ status }: { status: FacebookPostStatus }) {
  const busy = status === "drafting" || status === "publishing";
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium",
        STATUS_CLASS[status]
      )}
    >
      {busy ? (
        <Loader2 className="h-3 w-3 animate-spin" />
      ) : status === "published" ? (
        <CheckCircle2 className="h-3 w-3" />
      ) : status === "failed" ? (
        <AlertTriangle className="h-3 w-3" />
      ) : status === "scheduled" ? (
        <CalendarClock className="h-3 w-3" />
      ) : null}
      {FACEBOOK_STATUS_LABEL[status]}
    </span>
  );
}

/** ISO instant → the value a datetime-local input wants, in the browser's zone. */
function toLocalInput(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** datetime-local value → ISO instant, or undefined when empty or invalid. */
function fromLocalInput(v: string): string | undefined {
  if (!v) return undefined;
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

function ConnectionCard({ status }: { status: FacebookStatus }) {
  const connect = useConnectFacebook();
  const disconnect = useDisconnectFacebook();

  if (!status.configured) {
    return (
      <p className="rounded-md border border-signal/30 bg-signal/5 px-3 py-2 text-xs text-muted-foreground">
        Facebook is not configured on this deployment. Drafts are still written, but publishing needs a
        Meta app (FACEBOOK_APP_ID, FACEBOOK_APP_SECRET and FACEBOOK_TOKEN_KEY).
      </p>
    );
  }
  const reauth = status.pages.some((p) => p.needs_reauth);
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card p-4 shadow-card">
        <div className="min-w-0 text-sm">
          {status.connected ? (
            <>
              <p className="font-medium text-foreground">
                Connected as {status.name || "your Facebook account"} · {status.pages.length} Page
                {status.pages.length === 1 ? "" : "s"}
              </p>
              {reauth && (
                <p className="mt-0.5 text-xs text-signal-error">
                  Facebook rejected a Page&apos;s access. Reconnect to publish again.
                </p>
              )}
            </>
          ) : (
            <>
              <p className="font-medium text-foreground">Facebook is not connected</p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                Connect to choose the Pages to manage. Nothing is published until you approve it.
              </p>
            </>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => connect.mutate()}
            disabled={connect.isPending}
            className={status.connected && !reauth ? buttonSecondary : buttonPrimary}
          >
            <Spin on={connect.isPending}>
              <Link2 className="h-3.5 w-3.5" />
            </Spin>
            {status.connected ? "Reconnect" : "Connect Facebook"}
          </button>
          {status.connected && (
            <button
              type="button"
              disabled={disconnect.isPending}
              className={buttonSecondary}
              onClick={() => {
                if (
                  window.confirm(
                    "Disconnect Facebook? Drafts are kept; Page access is removed and publishing stops until you connect again."
                  )
                ) {
                  disconnect.mutate();
                }
              }}
            >
              Disconnect
            </button>
          )}
        </div>
      </div>
      {!status.connected && (
        <details className="rounded-md border border-border bg-card p-3 text-xs text-muted-foreground">
          <summary className="cursor-pointer font-medium text-foreground">
            Don&apos;t have a Facebook Page yet?
          </summary>
          <p className="mt-2">{status.page_creation.reason}</p>
          <ol className="mt-2 list-decimal space-y-1 pl-5">
            {status.page_creation.steps.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ol>
        </details>
      )}
    </div>
  );
}

/** One Page's brand profile and its details on Facebook. */
function PageCard({ page }: { page: FacebookPage }) {
  const [brief, setBrief] = useState(page.brief);
  const [voice, setVoice] = useState(page.voice);
  const [timezone, setTimezone] = useState(page.timezone);
  const [about, setAbout] = useState(page.about);
  const save = useSaveFacebookProfile();
  const refresh = useRefreshFacebookPage();
  const info = useUpdateFacebookPageInfo();
  const automation = useSetFacebookAutomation();

  const profileDirty = brief !== page.brief || voice !== page.voice || timezone !== page.timezone;
  return (
    <details className="rounded-xl border border-border bg-card p-4 shadow-card">
      <summary className="flex cursor-pointer flex-wrap items-center justify-between gap-2 text-sm font-medium text-foreground">
        <span>
          {page.name}
          <span className="ml-2 text-xs font-normal text-muted-foreground">
            {[page.category, `${page.followers.toLocaleString()} followers`].filter(Boolean).join(" · ")}
          </span>
        </span>
        {page.paused && <span className="text-2xs font-normal text-signal">Publishing paused</span>}
        {!page.brief && (
          <span className="text-2xs font-normal text-signal">Add a business brief for better posts</span>
        )}
      </summary>
      <div className="mt-3 grid gap-3 md:grid-cols-2">
        <label className="space-y-1 text-xs font-medium text-foreground md:col-span-2">
          Business brief
          <textarea
            value={brief}
            onChange={(e) => setBrief(e.target.value)}
            rows={4}
            placeholder="What the business does, who it serves, what makes it different. Only facts you are happy to see in posts."
            className={inputClass}
          />
        </label>
        <label className="space-y-1 text-xs font-medium text-foreground">
          Brand voice
          <input
            value={voice}
            onChange={(e) => setVoice(e.target.value)}
            placeholder="e.g. warm, practical, a little cheeky"
            className={inputClass}
          />
        </label>
        <label className="space-y-1 text-xs font-medium text-foreground">
          Timezone for posting times
          <input
            value={timezone}
            onChange={(e) => setTimezone(e.target.value)}
            placeholder="Europe/London"
            className={inputClass}
          />
        </label>
        <label className="space-y-1 text-xs font-medium text-foreground md:col-span-2">
          Page bio on Facebook
          <textarea value={about} onChange={(e) => setAbout(e.target.value)} rows={2} className={inputClass} />
        </label>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        <button
          type="button"
          className={buttonPrimary}
          disabled={!profileDirty || save.isPending}
          onClick={() => save.mutate({ pageId: page.page_id, brief, voice, timezone })}
        >
          <Spin on={save.isPending}>
            <Save className="h-3.5 w-3.5" />
          </Spin>
          Save profile
        </button>
        <button
          type="button"
          className={buttonSecondary}
          disabled={about === page.about || info.isPending}
          onClick={() => {
            if (window.confirm("Change this Page's bio on Facebook now?")) {
              info.mutate({ pageId: page.page_id, about });
            }
          }}
        >
          <Spin on={info.isPending}>
            <Send className="h-3.5 w-3.5" />
          </Spin>
          Update bio on Facebook
        </button>
        <button
          type="button"
          className={buttonSecondary}
          disabled={automation.isPending}
          onClick={() => automation.mutate({ pageId: page.page_id, paused: !page.paused })}
        >
          {page.paused ? "Resume publishing" : "Pause publishing"}
        </button>
        <button
          type="button"
          className={buttonSecondary}
          disabled={refresh.isPending}
          onClick={() => refresh.mutate(page.page_id)}
        >
          <Spin on={refresh.isPending}>
            <RefreshCw className="h-3.5 w-3.5" />
          </Spin>
          Refresh from Facebook
        </button>
      </div>
      <p className="mt-2 text-2xs text-muted-foreground">
        Name, profile picture and cover image can&apos;t be changed through Meta&apos;s API; change them in Facebook.
      </p>
    </details>
  );
}

function AnalyticsCard({ pageId }: { pageId: string }) {
  const [open, setOpen] = useState(false);
  const { data, isFetching, refetch } = useFacebookAnalytics(pageId, open);
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-card">
      <div className="flex items-center justify-between gap-2">
        <h3 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <BarChart3 className="h-4 w-4" /> Performance
        </h3>
        <button type="button" className={buttonSecondary} onClick={() => (open ? void refetch() : setOpen(true))}>
          <Spin on={isFetching}>
            <RefreshCw className="h-3.5 w-3.5" />
          </Spin>
          {open ? "Refresh" : "Load from Facebook"}
        </button>
      </div>
      {open && data && (
        <div className="mt-3 space-y-3 text-xs">
          {data.posts === 0 ? (
            <p className="text-muted-foreground">No published posts yet.</p>
          ) : (
            <>
              <dl className="grid grid-cols-2 gap-3 sm:grid-cols-5">
                {[
                  ["Posts", data.posts],
                  ["Reactions", data.reactions],
                  ["Comments", data.comments],
                  ["Shares", data.shares],
                  ["Avg / post", data.avg_engagement_per_post.toFixed(1)],
                ].map(([k, v]) => (
                  <div key={k}>
                    <dt className="text-muted-foreground">{k}</dt>
                    <dd className="text-base font-semibold text-foreground">{v}</dd>
                  </div>
                ))}
              </dl>
              {data.by_category.length > 0 && (
                <p className="text-muted-foreground">
                  By category (avg engagement):{" "}
                  {data.by_category.map((c) => `${c.category} ${c.avg_engagement_per_post.toFixed(1)}`).join(" · ")}.
                  The next calendar leans on what worked.
                </p>
              )}
              {data.top_post && (
                <p className="text-muted-foreground">
                  Top post: <span className="text-foreground">&ldquo;{data.top_post.message.slice(0, 90)}…&rdquo;</span>
                </p>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

function Comments({ post }: { post: FacebookPost }) {
  const [open, setOpen] = useState(false);
  const { data, isLoading } = useFacebookComments(post.id, open);
  const reply = useReplyToFacebookComment(post.id);
  const [draft, setDraft] = useState<Record<string, string>>({});
  return (
    <div className="mt-2">
      <button type="button" className={buttonSecondary} onClick={() => setOpen((o) => !o)}>
        <MessageCircle className="h-3.5 w-3.5" /> {open ? "Hide comments" : "Comments"}
      </button>
      {open && (
        <ul className="mt-2 space-y-2">
          {isLoading && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
          {data?.length === 0 && <li className="text-xs text-muted-foreground">No comments yet.</li>}
          {data?.map((c) => (
            <li key={c.id} className="rounded-md border border-border bg-background p-2 text-xs">
              <p>
                <span className="font-medium text-foreground">{c.from || "Someone"}</span>{" "}
                <span className="text-muted-foreground">{c.message}</span>
              </p>
              <div className="mt-1.5 flex gap-2">
                <input
                  value={draft[c.id] ?? ""}
                  onChange={(e) => setDraft((d) => ({ ...d, [c.id]: e.target.value }))}
                  placeholder="Reply as the Page…"
                  aria-label={`Reply to ${c.from}`}
                  className={inputClass}
                />
                <button
                  type="button"
                  className={buttonSecondary}
                  disabled={!draft[c.id]?.trim() || reply.isPending}
                  onClick={() =>
                    reply.mutate(
                      { commentId: c.id, message: draft[c.id] },
                      { onSuccess: () => setDraft((d) => ({ ...d, [c.id]: "" })) }
                    )
                  }
                >
                  Reply
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function PostCard({ post, canPublish }: { post: FacebookPost; canPublish: boolean }) {
  const [text, setText] = useState(post.message);
  const [when, setWhen] = useState(toLocalInput(post.scheduled_for));
  const update = useUpdateFacebookPost();
  const editLive = useEditPublishedFacebookPost();
  const approve = useApproveFacebookPost();
  const publish = usePublishFacebookPost();
  const unschedule = useUnscheduleFacebookPost();
  const redraft = useRedraftFacebookPost();
  const image = useGenerateFacebookImage();
  const del = useDeleteFacebookPost();

  const dirty = text !== post.message;
  // A redraft or refetch replaces the text unless the person is editing it.
  useEffect(() => {
    if (!dirty) setText(post.message);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [post.message]);
  useEffect(() => {
    setWhen(toLocalInput(post.scheduled_for));
  }, [post.scheduled_for]);

  const editable = post.status === "draft" || post.status === "failed" || post.status === "scheduled";
  const published = post.status === "published";
  const tooLong = text.length > FACEBOOK_MESSAGE_MAX;
  const whenISO = fromLocalInput(when);
  const busy = approve.isPending || publish.isPending;

  return (
    <div className="flex min-w-0 flex-col rounded-lg border border-border bg-background p-3">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">
          {[post.category, post.audience && `for ${post.audience}`].filter(Boolean).join(" · ") || "Post"}
        </span>
        <StatusBadge status={post.status} />
      </div>

      {post.image_url ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={post.image_url} alt="" className="mb-2 aspect-square w-full max-w-xs rounded-md object-cover" />
      ) : null}

      {post.status === "drafting" ? (
        <p className="py-6 text-center text-xs text-muted-foreground">Writing the post…</p>
      ) : (
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          readOnly={!editable && !published}
          rows={8}
          aria-label="Facebook post text"
          className={cn(inputClass, "resize-y leading-relaxed read-only:opacity-80")}
        />
      )}

      {post.image_prompt && !published && (
        <p className="mt-1.5 text-2xs text-muted-foreground">Creative idea: {post.image_prompt}</p>
      )}
      {post.error_message && post.status !== "drafting" && (
        <p className="mt-2 text-xs text-signal-error">{post.error_message}</p>
      )}

      {editable && (
        <label className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          Publish at
          <input
            type="datetime-local"
            value={when}
            onChange={(e) => setWhen(e.target.value)}
            className={cn(inputClass, "w-auto")}
          />
        </label>
      )}

      {published && (
        <p className="mt-2 text-xs text-muted-foreground">
          {post.reactions} reactions · {post.comments} comments · {post.shares} shares
        </p>
      )}

      <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
        <span className={cn("text-2xs", tooLong ? "text-signal-error" : "text-muted-foreground")}>
          {text.length.toLocaleString()} / {FACEBOOK_MESSAGE_MAX.toLocaleString()}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          {published && post.post_url && (
            <a
              href={post.post_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
            >
              View on Facebook <ExternalLink className="h-3 w-3" />
            </a>
          )}
          {published && dirty && (
            <button
              type="button"
              className={buttonSecondary}
              disabled={editLive.isPending || tooLong || !text.trim()}
              onClick={() => {
                if (window.confirm("Change this post's text on Facebook now?")) {
                  editLive.mutate({ id: post.id, message: text });
                }
              }}
            >
              <Spin on={editLive.isPending}>
                <Save className="h-3.5 w-3.5" />
              </Spin>
              Update on Facebook
            </button>
          )}
          {editable && dirty && (
            <button
              type="button"
              className={buttonSecondary}
              disabled={update.isPending || tooLong || !text.trim()}
              onClick={() => update.mutate({ id: post.id, message: text, scheduled_for: whenISO })}
            >
              <Spin on={update.isPending}>
                <Save className="h-3.5 w-3.5" />
              </Spin>
              Save
            </button>
          )}
          {(post.status === "draft" || post.status === "failed") && (
            <>
              {post.image_prompt && (
                <button
                  type="button"
                  className={buttonSecondary}
                  disabled={image.isPending}
                  onClick={() => image.mutate(post.id)}
                  title="Generate the creative from the idea above"
                >
                  <Spin on={image.isPending}>
                    <ImageIcon className="h-3.5 w-3.5" />
                  </Spin>
                  {post.image_url ? "New creative" : "Make creative"}
                </button>
              )}
              <button
                type="button"
                className={buttonSecondary}
                disabled={redraft.isPending}
                onClick={() => redraft.mutate(post.id)}
                title="Write this post again from the Page profile"
              >
                <Spin on={redraft.isPending}>
                  <RefreshCw className="h-3.5 w-3.5" />
                </Spin>
                Redraft
              </button>
            </>
          )}
          {post.status === "scheduled" && (
            <button
              type="button"
              className={buttonSecondary}
              disabled={unschedule.isPending}
              onClick={() => unschedule.mutate(post.id)}
            >
              Unschedule
            </button>
          )}
          {(post.status === "draft" || post.status === "failed") && (
            <>
              <button
                type="button"
                className={buttonSecondary}
                disabled={!canPublish || busy || dirty || !post.message.trim()}
                title={!canPublish ? "Connect Facebook to publish" : dirty ? "Save your edits first" : undefined}
                onClick={() => {
                  if (window.confirm("Publish this post to your Facebook Page now? It goes out publicly straight away.")) {
                    publish.mutate(post.id);
                  }
                }}
              >
                <Spin on={publish.isPending}>
                  <Send className="h-3.5 w-3.5" />
                </Spin>
                Publish now
              </button>
              <button
                type="button"
                className={buttonPrimary}
                disabled={!canPublish || busy || dirty || !whenISO || !post.message.trim()}
                title={
                  !canPublish
                    ? "Connect Facebook to schedule"
                    : dirty
                      ? "Save your edits first"
                      : !whenISO
                        ? "Choose a time"
                        : undefined
                }
                onClick={() => approve.mutate({ id: post.id, scheduledFor: whenISO })}
              >
                <Spin on={approve.isPending}>
                  <CalendarClock className="h-3.5 w-3.5" />
                </Spin>
                Approve &amp; schedule
              </button>
            </>
          )}
          {post.status !== "publishing" && post.status !== "drafting" && (
            <button
              type="button"
              className={buttonSecondary}
              disabled={del.isPending}
              aria-label="Delete post"
              onClick={() => {
                const live = published ? " It is deleted from your Facebook Page too." : "";
                if (window.confirm(`Delete this post?${live}`)) del.mutate(post.id);
              }}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>
      {published && <Comments post={post} />}
    </div>
  );
}

const SECTIONS: { title: string; statuses: FacebookPostStatus[] }[] = [
  { title: "To review", statuses: ["drafting", "draft", "failed"] },
  { title: "Scheduled", statuses: ["scheduled", "publishing"] },
  { title: "Published", statuses: ["published"] },
];

/**
 * Facebook Page Manager tab: the Facebook connection, each Page's brand
 * profile, and the content calendar to edit, approve, schedule and publish.
 * Planning a calendar starts from New task / Run task / chat, which use the
 * one server-side launch schema.
 */
export function FacebookClient() {
  const search = useSearchParams();
  const agentId = useParams<{ id?: string }>()?.id;
  const { data: status } = useFacebookStatus();
  const [selected, setSelected] = useState<string>("");
  const page: FacebookPage | undefined = useMemo(
    () => status?.pages.find((p) => p.page_id === selected) ?? status?.pages[0],
    [status, selected]
  );
  const { data: posts, isLoading } = useFacebookPosts(page?.page_id);
  const canPublish = Boolean(status?.configured && page && !page.needs_reauth);

  // Outcome of the Facebook consent redirect.
  useEffect(() => {
    if (search.get("facebook_connected") === "1") toast.success("Facebook connected");
    const err = search.get("facebook_error");
    if (err) toast.error(`Facebook connection failed: ${err}`);
  }, [search]);

  return (
    <div className="space-y-4">
      <p className="max-w-prose text-sm text-muted-foreground">
        Plans a Facebook content calendar for your Page, writes each post and its creative, and publishes
        at the times you approve. Nothing is posted until you press{" "}
        <span className="font-medium text-foreground">Approve &amp; schedule</span> or{" "}
        <span className="font-medium text-foreground">Publish now</span>.
      </p>

      {status && <ConnectionCard status={status} />}

      {status && status.pages.length > 1 && (
        <label className="flex items-center gap-2 text-xs font-medium text-foreground">
          Page
          <select
            value={page?.page_id ?? ""}
            onChange={(e) => setSelected(e.target.value)}
            className={cn(inputClass, "w-auto")}
          >
            {status.pages.map((p) => (
              <option key={p.page_id} value={p.page_id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
      )}

      {page && <PageCard key={page.page_id} page={page} />}
      {page && <AnalyticsCard key={`a-${page.page_id}`} pageId={page.page_id} />}

      {isLoading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : !posts || posts.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
          <CalendarClock className="mx-auto mb-2 h-6 w-6" />
          No posts yet. Plan a calendar from{" "}
          <Link href={newTaskHref(agentId)} className="font-medium text-primary hover:underline">
            New task
          </Link>
          {status?.connected ? "." : ", once a Page is connected."}
        </div>
      ) : (
        SECTIONS.map((s) => {
          const rows = posts.filter((p) => s.statuses.includes(p.status));
          if (rows.length === 0) return null;
          return (
            <section key={s.title} aria-label={s.title} className="space-y-2">
              <h3 className="text-sm font-semibold text-foreground">
                {s.title} <span className="font-normal text-muted-foreground">({rows.length})</span>
              </h3>
              <div className="grid gap-3 lg:grid-cols-2">
                {rows.map((p) => (
                  <PostCard key={p.id} post={p} canPublish={canPublish} />
                ))}
              </div>
            </section>
          );
        })
      )}
    </div>
  );
}
