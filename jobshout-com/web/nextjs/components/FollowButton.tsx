"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { toggleFollowAction } from "@/app/members/actions";
import { UserCheckIcon, UserPlusIcon } from "@/components/icons";
import { buttonClass, cx } from "@/components/ui";
import type { FollowTarget } from "@/lib/members";

/**
 * Follow a member or a showcase entry. Optimistic, like StarButton; signed
 * out it links to sign-in and comes back to `returnTo`.
 */
export function FollowButton({
  target,
  following,
  count,
  signedIn,
  returnTo,
  size = "md",
}: {
  target: FollowTarget;
  following: boolean;
  count: number;
  signedIn: boolean;
  returnTo: string;
  size?: "sm" | "md";
}) {
  const [state, setState] = useState({ following, count });
  const [error, setError] = useState("");
  const [pending, start] = useTransition();
  const label = target.type === "member" ? "followers" : "following for updates";

  if (!signedIn) {
    return (
      <Link
        href={`/login?callbackUrl=${encodeURIComponent(returnTo)}`}
        className={buttonClass("secondary", size)}
        title={`${count} ${label}`}
      >
        <UserPlusIcon className="h-4 w-4" />
        Follow
        <span className="text-mute">{count}</span>
      </Link>
    );
  }

  return (
    <span className="inline-flex flex-col items-start gap-1">
      <button
        type="button"
        aria-pressed={state.following}
        disabled={pending}
        title={`${state.count} ${label}`}
        onClick={() =>
          start(async () => {
            const on = !state.following;
            // Optimistic; rolled back if the API says no.
            setState({ following: on, count: Math.max(0, state.count + (on ? 1 : -1)) });
            setError("");
            const r = await toggleFollowAction(target, on);
            if (r.ok && r.follower_count !== undefined) {
              setState({ following: Boolean(r.following), count: r.follower_count });
            } else {
              setState(state);
              setError(r.message ?? "Could not update the follow.");
            }
          })
        }
        className={cx(buttonClass("secondary", size), state.following && "border-shout/40 bg-shout/10 text-ink")}
      >
        {state.following ? <UserCheckIcon className="h-4 w-4 text-shout" /> : <UserPlusIcon className="h-4 w-4" />}
        {state.following ? "Following" : "Follow"}
        <span className="text-mute">{state.count}</span>
      </button>
      {error ? (
        <span role="alert" className="text-xs font-medium text-shout">
          {error}
        </span>
      ) : null}
    </span>
  );
}
