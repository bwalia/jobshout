"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { joinGroupAction } from "@/app/groups/actions";
import { UserCheckIcon, UserPlusIcon } from "@/components/icons";
import { buttonClass } from "@/components/ui";

export function JoinButton({
  slug,
  isMember,
  joinPolicy,
  signedIn,
  returnTo,
}: {
  slug: string;
  isMember: boolean;
  joinPolicy: string;
  signedIn: boolean;
  returnTo: string;
}) {
  const [member, setMember] = useState(isMember);
  const [error, setError] = useState("");
  const [pending, start] = useTransition();

  if (member) {
    return (
      <span className={`${buttonClass("secondary", "md")} cursor-default`}>
        <UserCheckIcon className="h-4 w-4" />
        Member
      </span>
    );
  }

  if (!signedIn) {
    return (
      <Link
        href={`/login?callbackUrl=${encodeURIComponent(returnTo)}`}
        className={buttonClass("primary", "md")}
      >
        <UserPlusIcon className="h-4 w-4" />
        Sign in to join
      </Link>
    );
  }

  if (joinPolicy !== "open") {
    return (
      <span className="text-sm text-mute">
        This group is {joinPolicy === "invite" ? "invite-only" : "request-to-join"}.
      </span>
    );
  }

  return (
    <span className="inline-flex flex-col items-start gap-1">
      <button
        type="button"
        disabled={pending}
        className={buttonClass("primary", "md")}
        onClick={() =>
          start(async () => {
            setError("");
            const r = await joinGroupAction(slug);
            if (r.ok) setMember(true);
            else setError(r.message ?? "Could not join");
          })
        }
      >
        <UserPlusIcon className="h-4 w-4" />
        {pending ? "Joining…" : "Join group"}
      </button>
      {error ? <span className="text-sm text-warn">{error}</span> : null}
    </span>
  );
}
