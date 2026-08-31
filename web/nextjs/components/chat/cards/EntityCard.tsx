"use client";

import Link from "next/link";
import { cn } from "@/lib/utils/cn";
import type { EntityRef } from "@/lib/types/chat";
import { StoredImage } from "@/components/image/StoredImage";

export function EntityCard({ entity }: { entity: EntityRef }) {
  const href = entity.href || fallbackHref(entity);
  if (entity.kind === "image" && entity.url) {
    return (
      <Link
        href={href}
        className="block min-w-0 overflow-hidden rounded-lg border border-border bg-card transition-colors hover:border-primary/40"
      >
        <StoredImage
          src={entity.url}
          alt={entity.label || "generated image"}
          className="max-h-80 w-full bg-muted object-contain"
        />
        <div className="px-3 py-2 text-sm">
          <p className="text-[10px] font-mono uppercase tracking-wider text-muted-foreground">
            image
          </p>
          <p className="min-w-0 break-words font-medium text-foreground">{entity.label}</p>
        </div>
      </Link>
    );
  }
  return (
    <Link
      href={href}
      className="block min-w-0 rounded-lg border border-border bg-card px-3 py-2 text-sm transition-colors hover:border-primary/40 hover:bg-secondary/40"
    >
      <p className="text-[10px] font-mono uppercase tracking-wider text-muted-foreground">
        {entity.kind.replace(/_/g, " ")}
      </p>
      <p className="min-w-0 break-words font-medium text-foreground">{entity.label}</p>
    </Link>
  );
}

function fallbackHref(entity: EntityRef): string {
  switch (entity.kind) {
    case "agent":
      return entity.id
        ? `/panel/task-manager?agent=${entity.id}`
        : "/panel/task-manager";
    case "task":
      return entity.id
        ? `/panel/task-board?task=${entity.id}`
        : "/panel/task-board";
    case "project":
      return entity.id
        ? `/panel/task-manager?project=${entity.id}`
        : "/panel/task-manager";
    case "workflow":
    case "workflow_run":
      return entity.id ? `/workflows/${entity.id}` : "/panel/workflows";
    case "article_run":
      return entity.id ? `/articles/${entity.id}` : "/panel/artifacts";
    case "pentest_run":
      return entity.id
        ? `/panel/task-manager?agent=pentest&run=${entity.id}`
        : "/panel/task-manager?agent=pentest";
    case "review_run":
      return entity.id
        ? `/panel/task-manager?agent=review&run=${entity.id}`
        : "/panel/task-manager?agent=review";
    case "mail_thread":
      return entity.id
        ? `/panel/task-manager?agent=mail&thread=${entity.id}`
        : "/panel/task-manager?agent=mail";
    case "image":
      return "/panel/artifacts?kind=image";
    case "sprint":
      return "/panel/sprints";
    default:
      return "/panel/dashboard";
  }
}

export function EntityCardList({ entities }: { entities: EntityRef[] }) {
  if (!entities?.length) return null;
  const seen = new Set<string>();
  const unique = entities.filter((e) => {
    const k = `${e.kind}:${e.id}:${e.label}`;
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });
  return (
    <div className={cn("mt-2 grid min-w-0 gap-2", unique.length > 1 && "sm:grid-cols-2")}>
      {unique.map((e) => (
        <EntityCard key={`${e.kind}-${e.id}-${e.label}`} entity={e} />
      ))}
    </div>
  );
}
