"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { toggleStarAction } from "@/app/showcase/actions";
import { StarIcon } from "@/components/icons";
import { buttonClass, cx } from "@/components/ui";

export function StarButton({
  slug,
  starred,
  count,
  signedIn,
}: {
  slug: string;
  starred: boolean;
  count: number;
  signedIn: boolean;
}) {
  const [state, setState] = useState({ starred, count });
  const [error, setError] = useState("");
  const [pending, start] = useTransition();

  if (!signedIn) {
    return (
      <Link
        href={`/login?callbackUrl=${encodeURIComponent(`/showcase/${slug}`)}`}
        className={buttonClass("secondary", "md")}
      >
        <StarIcon className="h-4 w-4" />
        Star
        <span className="text-mute">{count}</span>
      </Link>
    );
  }

  return (
    <span className="inline-flex flex-col items-start gap-1">
      <button
        type="button"
        aria-pressed={state.starred}
        disabled={pending}
        onClick={() =>
          start(async () => {
            const on = !state.starred;
            // Optimistic; rolled back if the API says no.
            setState({ starred: on, count: state.count + (on ? 1 : -1) });
            setError("");
            const r = await toggleStarAction(slug, on);
            if (r.ok && r.star_count !== undefined) {
              setState({ starred: Boolean(r.starred), count: r.star_count });
            } else {
              setState(state);
              setError(r.message ?? "Could not update the star.");
            }
          })
        }
        className={cx(
          buttonClass("secondary", "md"),
          state.starred && "border-shout/40 bg-shout/10 text-ink",
        )}
      >
        <StarIcon className={cx("h-4 w-4", state.starred && "fill-current text-shout")} />
        {state.starred ? "Starred" : "Star"}
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
