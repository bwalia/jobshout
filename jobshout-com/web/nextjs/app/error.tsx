"use client";

import Link from "next/link";
import { useEffect } from "react";
import { Button, buttonClass } from "@/components/ui";
import { AlertIcon } from "@/components/icons";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="mx-auto flex min-h-[60vh] max-w-lg flex-col items-center justify-center px-5 py-24 text-center sm:px-8">
      <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-shout/10 text-shout">
        <AlertIcon className="h-7 w-7" />
      </span>
      <h1 className="mt-5 font-display text-3xl font-semibold tracking-[-0.025em] text-ink">
        Something broke on our side
      </h1>
      <p className="mt-3 text-base leading-relaxed text-mute">
        The page could not finish loading. Trying again usually sorts it — if the marketplace API
        is down locally, start it on port 8088.
      </p>
      <div className="mt-8 flex flex-wrap justify-center gap-3">
        <Button onClick={reset} size="md">
          Try again
        </Button>
        <Link href="/jobs" className={buttonClass("secondary", "md")}>
          Back to the board
        </Link>
      </div>
    </div>
  );
}
