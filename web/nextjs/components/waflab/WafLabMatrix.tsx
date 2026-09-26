"use client";

import { Fragment, useMemo, useState } from "react";
import { Loader2 } from "lucide-react";
import type { WAFLabResult, WAFLabScore } from "@/lib/api/waf-lab";

interface WafLabMatrixProps {
  results: WAFLabResult[];
  /** Run still in flight — an empty matrix means "not yet", not "none". */
  running?: boolean;
  score?: WAFLabScore | null;
  secureHost: string;
  openHost: string;
}

function verdictClass(verdict: string, blocked: boolean): string {
  if (verdict === "blocked" || (blocked && verdict !== "false_positive")) {
    return "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300";
  }
  if (verdict === "leaked" || verdict === "unexpected_block") {
    return "bg-red-500/15 text-red-700 dark:text-red-300";
  }
  if (verdict === "false_positive") {
    return "bg-amber-500/15 text-amber-800 dark:text-amber-200";
  }
  return "bg-muted text-muted-foreground";
}

export function WafLabMatrix({ results, running, score, secureHost, openHost }: WafLabMatrixProps) {
  const [expanded, setExpanded] = useState<string | null>(null);

  const rows = useMemo(() => {
    const byAttack = new Map<
      string,
      { meta: WAFLabResult; secure?: WAFLabResult; open?: WAFLabResult }
    >();
    for (const r of results) {
      let row = byAttack.get(r.attack_id);
      if (!row) {
        row = { meta: r };
        byAttack.set(r.attack_id, row);
      }
      if (r.host_role === "secure") row.secure = r;
      if (r.host_role === "open") row.open = r;
    }
    return Array.from(byAttack.values());
  }, [results]);

  if (results.length === 0) {
    return running ? (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin text-primary" />
        Attacks start once provisioning finishes — results appear here as they land.
      </p>
    ) : (
      <p className="text-sm text-muted-foreground">No attack results yet for this run.</p>
    );
  }

  return (
    <div className="space-y-4">
      {score && (
        <div className="grid gap-3 sm:grid-cols-4">
          <ScoreChip
            label="Secure blocked"
            value={`${score.secure_blocked}/${score.secure_expected}`}
          />
          <ScoreChip label="Open leaked" value={`${score.open_leaked}/${score.open_expected_leak}`} />
          <ScoreChip label="False positives" value={String(score.false_positives)} />
          <ScoreChip label="Not blocked (expected)" value={String(score.not_blocked_expected)} />
        </div>
      )}
      {score?.suspicious && (
        <p className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm text-amber-900 dark:text-amber-100">
          {score.suspicious_reason || "Score looks suspicious — inspect raw responses."}
        </p>
      )}
      {score?.bola_note && (
        <p className="text-xs text-muted-foreground">{score.bola_note}</p>
      )}

      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead className="bg-muted text-left text-xs text-muted-foreground">
            <tr>
              <th className="px-3 py-2 font-medium">Attack</th>
              <th className="px-3 py-2 font-medium">Category</th>
              <th className="px-3 py-2 font-medium" title={secureHost}>
                Secure
              </th>
              <th className="px-3 py-2 font-medium" title={openHost}>
                Open
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ meta, secure, open }) => {
              const openRow = expanded === meta.attack_id;
              return (
                <Fragment key={meta.attack_id}>
                  <tr
                    className="cursor-pointer border-t border-border hover:bg-muted/40"
                    onClick={() => setExpanded(openRow ? null : meta.attack_id)}
                  >
                    <td className="px-3 py-2 font-medium text-foreground">{meta.attack_name}</td>
                    <td className="px-3 py-2 text-muted-foreground">{meta.category}</td>
                    <td className="px-3 py-2">
                      <Cell result={secure} />
                    </td>
                    <td className="px-3 py-2">
                      <Cell result={open} />
                    </td>
                  </tr>
                  {openRow && (
                    <tr className="border-t border-border bg-muted/20">
                      <td colSpan={4} className="px-3 py-3 text-xs text-muted-foreground">
                        <Detail result={secure} label={`Secure (${secureHost})`} />
                        <Detail result={open} label={`Open (${openHost})`} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ScoreChip({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border bg-card px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-lg font-semibold tabular-nums text-foreground">{value}</div>
    </div>
  );
}

function Cell({ result }: { result?: WAFLabResult }) {
  if (!result) return <span className="text-muted-foreground">—</span>;
  return (
    <span
      className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${verdictClass(result.verdict, result.blocked)}`}
    >
      {result.verdict || (result.blocked ? "blocked" : "leaked")} · {result.status_code || "—"}
    </span>
  );
}

function Detail({ result, label }: { result?: WAFLabResult; label: string }) {
  if (!result) return null;
  return (
    <div className="mb-3 last:mb-0">
      <div className="mb-1 font-medium text-foreground">{label}</div>
      <div className="grid gap-1 sm:grid-cols-2">
        <span>
          {result.method} {result.path}
        </span>
        <span>rule: {result.waf_rule || "—"}</span>
        <span>violation: {result.waf_violation || "—"}</span>
        <span>support: {result.support_id || "—"}</span>
        <span>latency: {result.latency_ms}ms</span>
        {result.payload ? <span className="sm:col-span-2 break-all">payload: {result.payload}</span> : null}
        {result.notes ? <span className="sm:col-span-2">notes: {result.notes}</span> : null}
      </div>
    </div>
  );
}
