"use client";

import type { LLMCall } from "@/lib/types/llm-benchmark";
import { fmtMs, fmtTokens } from "./format";

const STATUS_CLS: Record<string, string> = {
  success: "text-signal-live",
  failed: "text-signal-error",
  cancelled: "text-muted-foreground",
};

/** One line per HTTP attempt: status, time and any error. */
function attemptsTitle(c: LLMCall): string | undefined {
  if (!c.api_attempts?.length) return undefined;
  return c.api_attempts
    .map(
      (a, i) =>
        `#${i + 1} ${a.http_status ?? "no response"} · ${fmtMs(a.duration_ms)}` +
        (a.request_id ? ` · ${a.request_id}` : "") +
        (a.error ? ` · ${a.error}` : "")
    )
    .join("\n");
}

/** Read-only list of individual LLM calls, in the order they were made. */
export function LLMCallsTable({ calls }: { calls: LLMCall[] }) {
  if (calls.length === 0) {
    return <p className="text-sm text-muted-foreground">No LLM calls recorded.</p>;
  }
  return (
    <div className="overflow-x-auto rounded-md border border-border scrollbar-thin">
      <table className="w-full text-left text-xs" data-testid="llm-calls-table">
        <thead className="bg-muted/50 text-muted-foreground">
          <tr>
            <th className="px-2 py-1.5 font-medium">#</th>
            <th className="px-2 py-1.5 font-medium">Stage</th>
            <th className="px-2 py-1.5 font-medium">Provider</th>
            <th className="px-2 py-1.5 font-medium">Model</th>
            <th className="px-2 py-1.5 text-right font-medium" title="The whole call, retries and backoff included">
              Duration
            </th>
            <th className="px-2 py-1.5 text-right font-medium" title="Time spent in HTTP exchanges with the provider">
              API time
            </th>
            <th className="px-2 py-1.5 text-right font-medium">In tok</th>
            <th className="px-2 py-1.5 text-right font-medium">Out tok</th>
            <th className="px-2 py-1.5 text-right font-medium" title="As reported by the provider">
              Total tok
            </th>
            <th className="px-2 py-1.5 text-right font-medium" title="HTTP requests actually sent to the provider">
              API calls
            </th>
            <th className="px-2 py-1.5 font-medium">Status</th>
          </tr>
        </thead>
        <tbody className="font-mono">
          {calls.map((c, i) => (
            <tr key={c.id} className="border-t border-border align-top">
              <td className="px-2 py-1.5 text-muted-foreground">{i + 1}</td>
              <td className="px-2 py-1.5">
                {c.stage || "—"}
                {c.attempt > 1 && (
                  <span className="ml-1 text-muted-foreground">(attempt {c.attempt})</span>
                )}
              </td>
              <td className="px-2 py-1.5">{c.provider}</td>
              <td
                className="px-2 py-1.5"
                title={c.provider_request_id ? `Provider request ID: ${c.provider_request_id}` : undefined}
              >
                {c.model || "—"}
                {!c.model_reported && c.model && (
                  <span className="ml-1 font-sans text-[11px] text-muted-foreground">(not reported)</span>
                )}
                {c.requested_model && c.requested_model !== c.model && (
                  <div className="font-sans text-[11px] text-muted-foreground">requested {c.requested_model}</div>
                )}
              </td>
              <td className="px-2 py-1.5 text-right">{fmtMs(c.duration_ms)}</td>
              <td className="px-2 py-1.5 text-right">{fmtMs(c.api_duration_ms)}</td>
              <td className="px-2 py-1.5 text-right" title={c.input_tokens === null ? "Not reported by the provider" : undefined}>
                {fmtTokens(c.input_tokens)}
              </td>
              <td className="px-2 py-1.5 text-right" title={c.output_tokens === null ? "Not reported by the provider" : undefined}>
                {fmtTokens(c.output_tokens)}
              </td>
              <td className="px-2 py-1.5 text-right" title={c.total_tokens === null ? "Not reported by the provider" : undefined}>
                {fmtTokens(c.total_tokens)}
              </td>
              <td className="px-2 py-1.5 text-right" title={attemptsTitle(c)}>
                {c.api_attempt_count ?? "—"}
              </td>
              <td className={"px-2 py-1.5 " + (STATUS_CLS[c.status] ?? "")}>
                {c.status}
                {c.error && (
                  <div className="max-w-xs whitespace-pre-wrap break-words font-sans text-[11px] text-muted-foreground">
                    {c.error}
                  </div>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
