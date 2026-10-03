"use client";

import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import { toast } from "sonner";
import {
  AlertTriangle,
  CheckCircle2,
  ExternalLink,
  Link2,
  Loader2,
  Newspaper,
  RefreshCw,
  Save,
  Send,
} from "lucide-react";
import {
  useConnectLinkedIn,
  useDisconnectLinkedIn,
  useLinkedInPosts,
  useLinkedInStatus,
  usePublishLinkedInPost,
  useRedraftLinkedInPost,
  useUpdateLinkedInPost,
} from "@/lib/hooks/useLinkedIn";
import { cn } from "@/lib/utils/cn";
import {
  LINKEDIN_COMMENTARY_MAX,
  LINKEDIN_VARIANT_LABEL,
  type LinkedInPost,
  type LinkedInPostStatus,
  type LinkedInStatus,
} from "@/lib/types/linkedin";

const STATUS_META: Record<LinkedInPostStatus, { label: string; className: string }> = {
  drafting: { label: "Drafting", className: "bg-signal-live/10 text-signal-live" },
  draft: { label: "Draft", className: "bg-signal/10 text-signal" },
  posting: { label: "Posting", className: "bg-signal-live/10 text-signal-live" },
  posted: { label: "Posted", className: "bg-status-done/10 text-status-done" },
  failed: { label: "Failed", className: "bg-signal-error/10 text-signal-error" },
};

function StatusBadge({ status }: { status: LinkedInPostStatus }) {
  const meta = STATUS_META[status];
  const busy = status === "drafting" || status === "posting";
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium",
        meta.className
      )}
    >
      {busy ? (
        <Loader2 className="h-3 w-3 animate-spin" />
      ) : status === "posted" ? (
        <CheckCircle2 className="h-3 w-3" />
      ) : status === "failed" ? (
        <AlertTriangle className="h-3 w-3" />
      ) : null}
      {meta.label}
    </span>
  );
}

const buttonBase =
  "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-opacity disabled:cursor-not-allowed disabled:opacity-50";
const buttonPrimary = cn(buttonBase, "bg-primary text-primary-foreground hover:opacity-90");
const buttonSecondary = cn(
  buttonBase,
  "border border-border bg-card text-foreground transition-colors hover:bg-muted"
);

/** Who the org posts as, and the connect / reconnect / disconnect actions. */
function ConnectionCard({ status }: { status: LinkedInStatus }) {
  const connect = useConnectLinkedIn();
  const disconnect = useDisconnectLinkedIn();

  if (!status.configured) {
    return (
      <p className="rounded-md border border-signal/30 bg-signal/5 px-3 py-2 text-xs text-muted-foreground">
        LinkedIn is not configured on this deployment. Drafts are still written, but posting needs a
        LinkedIn app (LINKEDIN_CLIENT_ID, LINKEDIN_CLIENT_SECRET and LINKEDIN_TOKEN_KEY).
      </p>
    );
  }

  const expiringSoon = status.connected && !status.expired && status.days_left <= 10;
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card p-4 shadow-card">
      <div className="min-w-0 text-sm">
        {status.connected ? (
          <>
            <p className="font-medium text-foreground">
              {status.expired ? "LinkedIn connection expired" : `Posting as ${status.name || "your LinkedIn profile"}`}
            </p>
            <p
              className={cn(
                "mt-0.5 text-xs",
                status.expired || expiringSoon ? "text-signal-error" : "text-muted-foreground"
              )}
            >
              {status.expired
                ? "LinkedIn gives a 60-day connection. Reconnect to post again."
                : `Connection lasts ${status.days_left} more day${status.days_left === 1 ? "" : "s"}; LinkedIn then asks you to reconnect.`}
            </p>
          </>
        ) : (
          <>
            <p className="font-medium text-foreground">LinkedIn is not connected</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              Connect your profile to post approved drafts
              {status.auto_draft ? " and to get drafts automatically when an article is published" : ""}.
            </p>
          </>
        )}
      </div>
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => connect.mutate()}
          disabled={connect.isPending}
          className={status.connected && !status.expired && !expiringSoon ? buttonSecondary : buttonPrimary}
        >
          {connect.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Link2 className="h-3.5 w-3.5" />}
          {status.connected ? "Reconnect" : "Connect LinkedIn"}
        </button>
        {status.connected && (
          <button
            type="button"
            onClick={() => {
              if (window.confirm("Disconnect LinkedIn? Drafts are kept; posting stops until you connect again.")) {
                disconnect.mutate();
              }
            }}
            disabled={disconnect.isPending}
            className={buttonSecondary}
          >
            Disconnect
          </button>
        )}
      </div>
    </div>
  );
}

/** One variant's draft: edit, approve and post, or redraft. */
function PostCard({ post, canPost }: { post: LinkedInPost; canPost: boolean }) {
  const [text, setText] = useState(post.commentary);
  const update = useUpdateLinkedInPost();
  const publish = usePublishLinkedInPost();
  const redraft = useRedraftLinkedInPost();

  // A redraft or a refetch replaces the text unless the person is editing it.
  const dirty = text !== post.commentary;
  useEffect(() => {
    if (!dirty) setText(post.commentary);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [post.commentary]);

  const editable = post.status === "draft" || post.status === "failed";
  const length = text.length;
  const tooLong = length > LINKEDIN_COMMENTARY_MAX;

  const onPost = () => {
    const ok = window.confirm(
      `Post the ${LINKEDIN_VARIANT_LABEL[post.variant].toLowerCase()} draft for "${post.article_title}" to LinkedIn now?\n\nIt goes out publicly on your profile straight away.`
    );
    if (ok) publish.mutate(post.id);
  };

  return (
    <div className="flex min-w-0 flex-col rounded-lg border border-border bg-background p-3">
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="text-xs font-semibold text-foreground">{LINKEDIN_VARIANT_LABEL[post.variant]}</span>
        <StatusBadge status={post.status} />
      </div>

      {post.status === "drafting" ? (
        <p className="flex-1 py-6 text-center text-xs text-muted-foreground">Writing the draft…</p>
      ) : (
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          readOnly={!editable}
          rows={12}
          aria-label={`${LINKEDIN_VARIANT_LABEL[post.variant]} LinkedIn post`}
          className="w-full flex-1 resize-y rounded-md border border-border bg-card p-2 text-xs leading-relaxed text-foreground focus:outline-none focus:ring-1 focus:ring-primary read-only:opacity-80"
        />
      )}

      {post.error_message && post.status !== "drafting" && (
        <p className="mt-2 text-xs text-signal-error">{post.error_message}</p>
      )}

      <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
        <span className={cn("text-2xs", tooLong ? "text-signal-error" : "text-muted-foreground")}>
          {length.toLocaleString()} / {LINKEDIN_COMMENTARY_MAX.toLocaleString()}
          {post.link_url ? " · article attached as a link card" : " · no public link yet"}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          {post.status === "posted" && post.post_url && (
            <a
              href={post.post_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
            >
              View on LinkedIn
              <ExternalLink className="h-3 w-3" />
            </a>
          )}
          {editable && dirty && (
            <button
              type="button"
              onClick={() => update.mutate({ id: post.id, commentary: text })}
              disabled={update.isPending || tooLong || !text.trim()}
              className={buttonSecondary}
            >
              {update.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
              Save
            </button>
          )}
          {editable && (
            <button
              type="button"
              onClick={() => redraft.mutate(post.id)}
              disabled={redraft.isPending}
              className={buttonSecondary}
              title="Write this draft again from the article"
            >
              {redraft.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
              Redraft
            </button>
          )}
          {post.status === "draft" && (
            <button
              type="button"
              onClick={onPost}
              disabled={!canPost || dirty || publish.isPending || !post.commentary.trim()}
              title={
                !canPost ? "Connect LinkedIn to post" : dirty ? "Save your edits first" : undefined
              }
              className={buttonPrimary}
            >
              {publish.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
              Approve &amp; post
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

interface ArticleGroup {
  articleId: string;
  title: string;
  linkURL: string;
  createdAt: string;
  posts: LinkedInPost[];
}

function groupByArticle(posts: LinkedInPost[]): ArticleGroup[] {
  const groups = new Map<string, ArticleGroup>();
  for (const p of posts) {
    let g = groups.get(p.article_id);
    if (!g) {
      g = { articleId: p.article_id, title: p.article_title, linkURL: p.link_url, createdAt: p.created_at, posts: [] };
      groups.set(p.article_id, g);
    }
    g.posts.push(p);
    if (p.created_at > g.createdAt) g.createdAt = p.created_at;
  }
  const order = { technical: 0, business: 1 } as const;
  return Array.from(groups.values())
    .map((g) => ({ ...g, posts: g.posts.sort((a, b) => order[a.variant] - order[b.variant]) }))
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

/**
 * LinkedIn Poster tab: the LinkedIn connection, and each article's technical
 * and business drafts to edit, approve and post. Drafting for a chosen article
 * starts from New task / Run task / chat, which use the one server-side
 * launch schema; newly published articles are drafted automatically.
 */
export function LinkedInClient() {
  const search = useSearchParams();
  const { data: status } = useLinkedInStatus();
  const { data: posts, isLoading } = useLinkedInPosts();
  const groups = useMemo(() => groupByArticle(posts ?? []), [posts]);
  const canPost = Boolean(status?.configured && status.connected && !status.expired);

  // Outcome of the LinkedIn consent redirect.
  useEffect(() => {
    if (search.get("linkedin_connected") === "1") toast.success("LinkedIn connected");
    const err = search.get("linkedin_error");
    if (err) toast.error(`LinkedIn connection failed: ${err}`);
  }, [search]);

  return (
    <div className="space-y-4">
      <p className="max-w-prose text-sm text-muted-foreground">
        Turns published articles into LinkedIn posts: a <span className="font-medium text-foreground">technical</span>{" "}
        one for engineers and a <span className="font-medium text-foreground">business</span> one for decision
        makers, each led by the problem the article solves. Nothing is posted until you press{" "}
        <span className="font-medium text-foreground">Approve &amp; post</span>.
      </p>

      {status && <ConnectionCard status={status} />}

      {isLoading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : groups.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
          <Newspaper className="mx-auto mb-2 h-6 w-6" />
          No drafts yet. Launch the LinkedIn Poster from <span className="font-medium">New task</span> for a
          published article{status?.auto_draft ? ", or connect LinkedIn and new articles are drafted when they are published" : ""}.
        </div>
      ) : (
        <ul className="space-y-3" aria-label="LinkedIn drafts">
          {groups.map((g) => (
            <li key={g.articleId} className="rounded-xl border border-border bg-card p-4 shadow-card">
              <div className="mb-3 flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <h3 className="line-clamp-2 text-sm font-semibold text-foreground">{g.title}</h3>
                  <p className="mt-1 text-xs text-muted-foreground">{new Date(g.createdAt).toLocaleString()}</p>
                </div>
                {g.linkURL && (
                  <a
                    href={g.linkURL}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
                  >
                    Article
                    <ExternalLink className="h-3 w-3" />
                  </a>
                )}
              </div>
              <div className="grid gap-3 md:grid-cols-2">
                {g.posts.map((p) => (
                  <PostCard key={p.id} post={p} canPost={canPost} />
                ))}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
