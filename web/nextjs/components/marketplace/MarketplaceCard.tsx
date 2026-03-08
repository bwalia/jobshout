"use client";

interface MarketplaceAgent {
  id: string;
  name: string;
  role: string;
  category: string;
  description: string;
  download_count: number;
  star_rating: number;
  author: string;
}

interface MarketplaceCardProps {
  agent: MarketplaceAgent;
  onImport: (agentId: string) => void;
}

// Maps category names to Tailwind background/text colour pairs for the badge
const CATEGORY_COLORS: Record<string, string> = {
  Engineering: "bg-blue-100 text-blue-700",
  Design: "bg-purple-100 text-purple-700",
  QA: "bg-green-100 text-green-700",
  Management: "bg-orange-100 text-orange-700",
  DevOps: "bg-red-100 text-red-700",
};

function formatDownloadCount(count: number): string {
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1)}k`;
  }
  return String(count);
}

export function MarketplaceCard({ agent, onImport }: MarketplaceCardProps) {
  const categoryColour =
    CATEGORY_COLORS[agent.category] ?? "bg-secondary text-secondary-foreground";

  // Build star display: filled stars up to Math.floor(rating), partial or empty remainder
  const fullStars = Math.floor(agent.star_rating);
  const hasHalfStar = agent.star_rating - fullStars >= 0.5;
  const emptyStars = 5 - fullStars - (hasHalfStar ? 1 : 0);

  return (
    <div className="flex flex-col rounded-xl border border-border bg-card p-5 shadow-sm transition-shadow hover:shadow-md">
      {/* Avatar placeholder + category badge row */}
      <div className="flex items-start justify-between">
        <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-xl font-bold text-primary">
          {agent.name.charAt(0)}
        </div>
        <span
          className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${categoryColour}`}
        >
          {agent.category}
        </span>
      </div>

      {/* Name and role */}
      <div className="mt-3">
        <h3 className="font-semibold leading-tight">{agent.name}</h3>
        <p className="mt-0.5 text-sm text-muted-foreground">{agent.role}</p>
      </div>

      {/* Description */}
      <p className="mt-2 line-clamp-3 flex-1 text-sm text-muted-foreground">
        {agent.description}
      </p>

      {/* Stats row */}
      <div className="mt-4 flex items-center gap-4 text-xs text-muted-foreground">
        {/* Download count */}
        <span className="flex items-center gap-1">
          <svg
            className="h-3.5 w-3.5"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M4 16v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2M7 10l5 5 5-5M12 15V3"
            />
          </svg>
          {formatDownloadCount(agent.download_count)}
        </span>

        {/* Star rating */}
        <span className="flex items-center gap-1">
          <span className="flex">
            {Array.from({ length: fullStars }).map((_, i) => (
              <svg
                key={`full-${i}`}
                className="h-3.5 w-3.5 fill-yellow-400 text-yellow-400"
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 0 0 .95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 0 0-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 0 0-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 0 0-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 0 0 .951-.69l1.07-3.292z" />
              </svg>
            ))}
            {hasHalfStar && (
              <svg
                key="half"
                className="h-3.5 w-3.5 fill-yellow-400 text-yellow-400 opacity-60"
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 0 0 .95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 0 0-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 0 0-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 0 0-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 0 0 .951-.69l1.07-3.292z" />
              </svg>
            )}
            {Array.from({ length: emptyStars }).map((_, i) => (
              <svg
                key={`empty-${i}`}
                className="h-3.5 w-3.5 fill-muted text-muted"
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 0 0 .95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 0 0-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 0 0-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 0 0-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 0 0 .951-.69l1.07-3.292z" />
              </svg>
            ))}
          </span>
          {agent.star_rating.toFixed(1)}
        </span>
      </div>

      {/* Author */}
      <p className="mt-2 text-xs text-muted-foreground">
        by <span className="font-medium">{agent.author}</span>
      </p>

      {/* Import button */}
      <button
        type="button"
        onClick={() => onImport(agent.id)}
        className="mt-4 inline-flex h-9 w-full items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        Import
      </button>
    </div>
  );
}
