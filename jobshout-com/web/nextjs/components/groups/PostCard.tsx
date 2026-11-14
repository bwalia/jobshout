"use client";

import Link from "next/link";
import { useEffect, useRef, useState, useTransition } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { addCommentAction, likePostAction, loadCommentsAction } from "@/app/groups/actions";
import { MessageIcon, StarIcon } from "@/components/icons";
import { Button, ErrorNotice, Field, Textarea, buttonClass } from "@/components/ui";
import { EMPTY_FORM_STATE } from "@/lib/form-state";
import type { GroupComment, GroupPost } from "@/lib/groups";

function CommentSubmit() {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="sm" disabled={pending}>
      {pending ? "…" : "Reply"}
    </Button>
  );
}

function CommentForm({
  slug,
  postId,
  onPosted,
}: {
  slug: string;
  postId: string;
  onPosted: () => void;
}) {
  const bound = addCommentAction.bind(null, slug, postId);
  const [state, action] = useFormState(bound, EMPTY_FORM_STATE);
  const formRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    if (state.ok) {
      formRef.current?.reset();
      onPosted();
    }
  }, [state, onPosted]);

  return (
    <form ref={formRef} action={action} className="mt-3 space-y-2">
      {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}
      <Field label="Comment" htmlFor={`comment-${postId}`} error={state.fieldErrors?.body}>
        <Textarea id={`comment-${postId}`} name="body" required rows={2} maxLength={5000} />
      </Field>
      <CommentSubmit />
    </form>
  );
}

export function PostCard({
  slug,
  post,
  isMember,
  signedIn,
  returnTo,
}: {
  slug: string;
  post: GroupPost;
  isMember: boolean;
  signedIn: boolean;
  returnTo: string;
}) {
  const [open, setOpen] = useState(false);
  const [comments, setComments] = useState<GroupComment[]>([]);
  const [commentCount, setCommentCount] = useState(post.comment_count);
  const [likes, setLikes] = useState(post.reaction_count);
  const [liked, setLiked] = useState(post.liked_by_me);
  const [err, setErr] = useState("");
  const [pending, start] = useTransition();
  const author = post.author_display_name || post.author_handle || "Member";

  function refreshComments() {
    start(async () => {
      const r = await loadCommentsAction(slug, post.id);
      if (r.ok && r.comments) {
        setComments(r.comments);
        setCommentCount(r.comments.length);
      }
    });
  }

  function toggleThread() {
    const next = !open;
    setOpen(next);
    if (next) refreshComments();
  }

  return (
    <article className="rounded-card border border-line bg-surface p-5 sm:p-6">
      <header className="flex flex-wrap items-baseline justify-between gap-2">
        <div>
          {post.title ? (
            <h3 className="font-display text-lg font-semibold text-ink">{post.title}</h3>
          ) : null}
          <p className="text-sm text-mute">
            {post.author_handle ? (
              <Link
                href={`/members/${post.author_handle}`}
                className="font-medium text-ink hover:text-shout"
              >
                {author}
              </Link>
            ) : (
              <span className="font-medium text-ink">{author}</span>
            )}
            <span className="mx-1.5">·</span>
            <time dateTime={post.created_at}>
              {new Date(post.created_at).toLocaleString(undefined, {
                dateStyle: "medium",
                timeStyle: "short",
              })}
            </time>
            {post.pinned ? (
              <span className="ml-2 text-xs font-semibold text-shout">Pinned</span>
            ) : null}
          </p>
        </div>
        <span className="text-xs uppercase tracking-wide text-mute">{post.kind}</span>
      </header>
      <p className="mt-3 whitespace-pre-wrap text-body leading-relaxed">{post.body}</p>
      <div className="mt-4 flex flex-wrap items-center gap-3">
        {signedIn && isMember ? (
          <button
            type="button"
            disabled={pending || liked}
            className={buttonClass(liked ? "secondary" : "ghost", "sm")}
            onClick={() =>
              start(async () => {
                setErr("");
                const r = await likePostAction(slug, post.id);
                if (r.ok) {
                  setLiked(true);
                  if (typeof r.reaction_count === "number") setLikes(r.reaction_count);
                  else setLikes((n) => n + 1);
                } else setErr(r.message ?? "Could not like");
              })
            }
          >
            <StarIcon className="h-4 w-4" />
            {liked ? "Liked" : "Like"}
            <span className="text-mute">{likes}</span>
          </button>
        ) : (
          <span className={`${buttonClass("ghost", "sm")} cursor-default`}>
            <StarIcon className="h-4 w-4" />
            {likes}
          </span>
        )}
        <button
          type="button"
          className={buttonClass("ghost", "sm")}
          onClick={toggleThread}
          aria-expanded={open}
        >
          <MessageIcon className="h-4 w-4" />
          {commentCount} {commentCount === 1 ? "comment" : "comments"}
        </button>
        {!signedIn ? (
          <Link
            href={`/login?callbackUrl=${encodeURIComponent(returnTo)}`}
            className="text-sm text-mute underline-offset-2 hover:text-ink hover:underline"
          >
            Sign in to join the conversation
          </Link>
        ) : null}
      </div>
      {err ? <p className="mt-2 text-sm text-warn">{err}</p> : null}
      {open ? (
        <div className="mt-4 border-t border-line pt-4">
          {comments.length === 0 ? (
            <p className="text-sm text-mute">No comments yet.</p>
          ) : (
            <ul className="space-y-3">
              {comments.map((c) => (
                <li key={c.id} className="text-sm">
                  <p className="font-medium text-ink">
                    {c.author_handle ? (
                      <Link href={`/members/${c.author_handle}`} className="hover:text-shout">
                        {c.author_display_name || c.author_handle}
                      </Link>
                    ) : (
                      c.author_display_name || "Member"
                    )}
                    <span className="ml-2 font-normal text-mute">
                      {new Date(c.created_at).toLocaleString(undefined, {
                        dateStyle: "medium",
                        timeStyle: "short",
                      })}
                    </span>
                  </p>
                  <p className="mt-1 whitespace-pre-wrap text-body">{c.body}</p>
                </li>
              ))}
            </ul>
          )}
          {signedIn && isMember && !post.locked ? (
            <CommentForm slug={slug} postId={post.id} onPosted={refreshComments} />
          ) : null}
        </div>
      ) : null}
    </article>
  );
}
