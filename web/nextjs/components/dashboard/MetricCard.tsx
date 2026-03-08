interface MetricCardProps {
  /** Short label displayed above the value */
  title: string;
  /** The primary metric value to display prominently */
  value: string;
  /**
   * Percentage change vs the previous period.
   * Positive values are shown in green with an up arrow;
   * negative values are shown in red with a down arrow.
   */
  delta: number;
  /** Optional explanatory text shown below the delta */
  description?: string;
}

export function MetricCard({ title, value, delta, description }: MetricCardProps) {
  const isPositive = delta >= 0;

  return (
    <div className="rounded-xl border border-border bg-card p-5 shadow-sm">
      {/* Title */}
      <p className="text-sm font-medium text-muted-foreground">{title}</p>

      {/* Large value */}
      <p className="mt-2 text-3xl font-bold tracking-tight">{value}</p>

      {/* Delta badge */}
      <div className="mt-2 flex items-center gap-1.5">
        <span
          className={[
            "inline-flex items-center gap-0.5 rounded-full px-2 py-0.5 text-xs font-semibold",
            isPositive
              ? "bg-green-100 text-green-700"
              : "bg-red-100 text-red-700",
          ].join(" ")}
        >
          {/* Arrow icon - points up for positive, down for negative */}
          <svg
            className="h-3 w-3"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={3}
          >
            {isPositive ? (
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 15l7-7 7 7" />
            ) : (
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            )}
          </svg>
          {Math.abs(delta).toFixed(1)}%
        </span>

        {description && (
          <span className="text-xs text-muted-foreground">{description}</span>
        )}
      </div>
    </div>
  );
}
