import { ArrowDownRight, ArrowUpRight } from "lucide-react";
import { cn } from "@/lib/utils/cn";

interface MetricCardProps {
  /** Short label displayed above the value */
  title: string;
  /** The primary metric value to display prominently */
  value: string;
  /**
   * Percentage change vs the previous period. Positive is shown as a live-green
   * signal, negative as an error-red one. Omitted when the API has no trend.
   */
  delta?: number;
  /** Optional explanatory text shown below the delta */
  description?: string;
}

/**
 * A telemetry KPI tile: mono, tabular value (the "ops console" voice), a
 * signal-coloured delta, and an amber signal hairline that lights on hover.
 */
export function MetricCard({ title, value, delta, description }: MetricCardProps) {
  const isPositive = (delta ?? 0) >= 0;

  return (
    <div className="group relative overflow-hidden rounded-xl border border-border bg-card p-5 transition-colors hover:border-primary/40">
      {/* Amber signal hairline — lights on hover, quiet otherwise. */}
      <span
        aria-hidden
        className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-primary to-transparent opacity-0 transition-opacity duration-300 group-hover:opacity-90"
      />

      <p className="text-2xs font-semibold uppercase tracking-wider text-muted-foreground">
        {title}
      </p>

      <p className="tabular mt-2.5 text-3xl font-semibold tracking-tight text-foreground">
        {value}
      </p>

      {(typeof delta === "number" || description) && (
        <div className="mt-2 flex items-center gap-2">
          {typeof delta === "number" && (
            <span
              className={cn(
                "tabular inline-flex items-center gap-0.5 rounded-md px-1.5 py-0.5 text-xs font-semibold",
                isPositive
                  ? "bg-signal-live/12 text-signal-live"
                  : "bg-signal-error/12 text-signal-error"
              )}
            >
              {isPositive ? (
                <ArrowUpRight className="h-3 w-3" />
              ) : (
                <ArrowDownRight className="h-3 w-3" />
              )}
              {Math.abs(delta).toFixed(1)}%
            </span>
          )}

          {description && (
            <span className="text-xs text-muted-foreground">{description}</span>
          )}
        </div>
      )}
    </div>
  );
}
