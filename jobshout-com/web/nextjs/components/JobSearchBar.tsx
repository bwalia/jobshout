"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Button } from "@/components/ui";
import { MapPinIcon, SearchIcon } from "@/components/icons";

/**
 * The hero CTA. Submits into /jobs as query params so every search is a
 * shareable URL and the board can render server-side.
 */
export function JobSearchBar({
  defaultQuery = "",
  defaultLocation = "",
  size = "lg",
}: {
  defaultQuery?: string;
  defaultLocation?: string;
  size?: "lg" | "md";
}) {
  const router = useRouter();
  const [q, setQ] = useState(defaultQuery);
  const [where, setWhere] = useState(defaultLocation);

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const params = new URLSearchParams();
    if (q.trim()) params.set("q", q.trim());
    if (where.trim()) params.set("where", where.trim());
    router.push(`/jobs${params.toString() ? `?${params}` : ""}`);
  }

  const tall = size === "lg";

  return (
    <form
      onSubmit={submit}
      role="search"
      className={`surface-card flex flex-col gap-2 p-2 shadow-card sm:flex-row sm:items-center ${
        tall ? "sm:gap-0" : ""
      }`}
    >
      <div className="flex flex-1 items-center gap-3 rounded-xl px-3 transition-shadow duration-200 focus-within:ring-2 focus-within:ring-shout/40">
        <SearchIcon className="h-5 w-5 shrink-0 text-mute" />
        <input
          type="search"
          name="q"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Job title, skill, or keyword"
          aria-label="Job title, skill, or keyword"
          className={`w-full bg-transparent text-[0.95rem] text-ink placeholder:text-mute/80 focus:outline-none ${
            tall ? "py-4" : "py-3"
          }`}
        />
      </div>

      <span aria-hidden className="mx-2 hidden h-7 w-px bg-line sm:block" />

      <div className="flex flex-1 items-center gap-3 rounded-xl px-3 transition-shadow duration-200 focus-within:ring-2 focus-within:ring-shout/40 sm:max-w-[15rem]">
        <MapPinIcon className="h-5 w-5 shrink-0 text-mute" />
        <input
          type="text"
          name="where"
          value={where}
          onChange={(e) => setWhere(e.target.value)}
          placeholder="City or remote"
          aria-label="City or remote"
          className={`w-full bg-transparent text-[0.95rem] text-ink placeholder:text-mute/80 focus:outline-none ${
            tall ? "py-4" : "py-3"
          }`}
        />
      </div>

      <Button type="submit" size={tall ? "lg" : "md"} className="shrink-0">
        Search jobs
      </Button>
    </form>
  );
}
