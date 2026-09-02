"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  AlertTriangle,
  Archive,
  CheckCircle2,
  Clock,
  ImageIcon,
  Newspaper,
  Plus,
  Search,
} from "lucide-react";
import { GenerateArticleDialog } from "@/components/blog/GenerateArticleDialog";
import { StoredImage } from "@/components/image/StoredImage";
import { SignalDot } from "@/components/ui/signal-dot";
import {
  artifactsFromBlogRuns,
  artifactsFromImages,
  sortArtifacts,
} from "@/lib/artifacts";
import { useBlogRuns } from "@/lib/hooks/useBlog";
import { useGeneratedImages } from "@/lib/hooks/useImages";
import {
  ARTIFACT_KINDS,
  type ArtifactFilter,
  type ArtifactItem,
} from "@/lib/types/artifact";
import { cn } from "@/lib/utils/cn";

const STATUS_META: Record<
  string,
  { label: string; icon: React.ElementType; className: string }
> = {
  pending: {
    label: "Queued",
    icon: Clock,
    className: "bg-status-todo/15 text-status-todo",
  },
  running: {
    label: "Writing",
    icon: Clock,
    className: "bg-status-progress/15 text-status-progress",
  },
  completed: {
    label: "Ready",
    icon: CheckCircle2,
    className: "bg-status-done/15 text-status-done",
  },
  failed: {
    label: "Failed",
    icon: AlertTriangle,
    className: "bg-status-blocked/15 text-status-blocked",
  },
  cancelled: {
    label: "Stopped",
    icon: Archive,
    className: "bg-muted text-muted-foreground",
  },
};

function parseFilter(value: string | null): ArtifactFilter {
  if (value && ARTIFACT_KINDS.some((k) => k.id === value)) {
    return value as ArtifactFilter;
  }
  return "all";
}

function StatusBadge({ status }: { status: string }) {
  const meta = STATUS_META[status] ?? STATUS_META.pending;
  const Icon = meta.icon;
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium",
        meta.className
      )}
    >
      <Icon className="h-3 w-3" />
      {meta.label}
    </span>
  );
}

function KindBadge({ kind }: { kind: ArtifactItem["kind"] }) {
  const label = ARTIFACT_KINDS.find((k) => k.id === kind)?.singular ?? kind;
  return (
    <span className="text-[10px] font-mono uppercase tracking-wider text-muted-foreground">
      {label}
    </span>
  );
}

function ArtifactCard({ item }: { item: ArtifactItem }) {
  const inner = (
    <>
      {item.kind === "image" && (
        <div className="mb-3 overflow-hidden rounded-lg bg-muted">
          {item.imageUrl ? (
            <StoredImage
              src={item.imageUrl}
              alt={item.title}
              loading="lazy"
              className="aspect-video w-full object-cover"
            />
          ) : (
            <div className="flex aspect-video w-full items-center justify-center text-xs text-muted-foreground">
              not stored
            </div>
          )}
        </div>
      )}
      <div className="flex items-start justify-between gap-3">
        <KindBadge kind={item.kind} />
        {item.status ? <StatusBadge status={item.status} /> : null}
      </div>
      <h3 className="mt-2 line-clamp-2 text-sm font-semibold text-foreground">
        {item.title}
      </h3>
      {item.subtitle && (
        <p className="mt-1.5 line-clamp-2 text-xs text-muted-foreground">
          {item.subtitle}
        </p>
      )}
      {item.status === "running" && item.meta && (
        <p className="mt-3 flex items-center gap-2 rounded bg-muted/60 px-2 py-1.5 text-2xs text-foreground/80">
          <SignalDot status="live" size="sm" />
          <span className="truncate">{item.meta}</span>
        </p>
      )}
      <div className="mt-auto flex items-center justify-between gap-3 pt-4 text-2xs text-muted-foreground">
        <span className="truncate">
          {item.status === "running" ? null : item.meta}
        </span>
        {item.createdAt && (
          <time dateTime={item.createdAt}>
            {new Date(item.createdAt).toLocaleDateString()}
          </time>
        )}
      </div>
    </>
  );

  const className =
    "group flex h-full flex-col rounded-xl border border-border bg-card p-5 shadow-card transition-shadow hover:shadow-card-hover";

  if (item.href && item.kind === "article") {
    return (
      <Link href={item.href} className={className}>
        {inner}
      </Link>
    );
  }

  return <div className={className}>{inner}</div>;
}

export function ArtifactsPanel() {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const filter = parseFilter(searchParams.get("kind"));
  const [searchQuery, setSearchQuery] = useState("");
  const [isCreateOpen, setIsCreateOpen] = useState(false);

  const blogs = useBlogRuns({ per_page: 50 });
  const images = useGeneratedImages(50);

  const items = useMemo(() => {
    const articleItems = artifactsFromBlogRuns(blogs.data?.data ?? []);
    const imageItems = images.isError
      ? []
      : artifactsFromImages(images.data ?? []);
    const merged = sortArtifacts([...articleItems, ...imageItems]);
    const byKind =
      filter === "all" ? merged : merged.filter((item) => item.kind === filter);
    const q = searchQuery.trim().toLowerCase();
    if (!q) return byKind;
    return byKind.filter((item) =>
      [item.title, item.subtitle, item.meta]
        .filter(Boolean)
        .some((field) => field!.toLowerCase().includes(q))
    );
  }, [blogs.data, images.data, images.isError, filter, searchQuery]);

  const loading = blogs.isLoading || (filter !== "article" && images.isLoading);

  function setFilter(next: ArtifactFilter) {
    const params = new URLSearchParams(searchParams.toString());
    if (next === "all") params.delete("kind");
    else params.set("kind", next);
    const qs = params.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground">
            Artifacts
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Articles and other work agents have produced. Open one to review it.
          </p>
        </div>
        <button
          type="button"
          onClick={() => setIsCreateOpen(true)}
          className="inline-flex shrink-0 items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <Plus className="h-4 w-4" />
          Write articles
        </button>
      </div>

      <div className="flex gap-1 border-b border-border">
        {(
          [
            { id: "all" as const, label: "All" },
            ...ARTIFACT_KINDS.map((k) => ({ id: k.id, label: k.label })),
          ] as const
        ).map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setFilter(tab.id)}
            className={cn(
              "border-b-2 px-4 py-2 text-sm font-medium transition-colors",
              filter === tab.id
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          type="search"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          placeholder="Search artifacts…"
          className="w-full rounded-md border border-border bg-background py-2 pl-9 pr-3 text-sm text-foreground placeholder:text-muted-foreground"
        />
      </div>

      {loading && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-40 animate-pulse rounded-xl bg-muted" />
          ))}
        </div>
      )}

      {blogs.isError && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          Failed to load articles.
        </div>
      )}

      {!loading && !blogs.isError && items.length === 0 && (
        <div className="rounded-xl border border-dashed border-border py-16 text-center">
          {filter === "image" ? (
            <ImageIcon className="mx-auto h-10 w-10 text-muted-foreground/50" />
          ) : (
            <Archive className="mx-auto h-10 w-10 text-muted-foreground/50" />
          )}
          <p className="mt-3 text-sm text-muted-foreground">
            {searchQuery
              ? "No artifacts match that search."
              : filter === "image"
                ? "No images yet."
                : filter === "article"
                  ? "No articles yet."
                  : "No artifacts yet."}
          </p>
          {!searchQuery && filter !== "image" && (
            <button
              type="button"
              onClick={() => setIsCreateOpen(true)}
              className="mt-4 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
            >
              <Newspaper className="h-4 w-4" />
              Write your first article
            </button>
          )}
        </div>
      )}

      {!loading && items.length > 0 && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {items.map((item) => (
            <ArtifactCard key={item.id} item={item} />
          ))}
        </div>
      )}

      <GenerateArticleDialog
        open={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
      />
    </div>
  );
}
