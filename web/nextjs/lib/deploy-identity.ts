/** Deploy identity for the sidebar / login footer. Values come from GET /health. */

export type DeployIdentity = {
  version?: string;
  env?: string;
  git_sha?: string;
  build_id?: string;
  deployed_at?: string;
};

/** Short 7-char SHA when a full commit hash is present. */
export function shortSha(sha?: string): string | undefined {
  const s = sha?.trim();
  if (!s) return undefined;
  return s.length > 7 ? s.slice(0, 7) : s;
}

/** Absolute deploy time in GMT, e.g. "13 Nov 2026, 17:38 GMT". */
export function formatDeployedAtGmt(iso?: string): string | undefined {
  if (!iso) return undefined;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return undefined;
  const day = d.getUTCDate();
  const months = [
    "Jan",
    "Feb",
    "Mar",
    "Apr",
    "May",
    "Jun",
    "Jul",
    "Aug",
    "Sep",
    "Oct",
    "Nov",
    "Dec",
  ];
  const mon = months[d.getUTCMonth()];
  const year = d.getUTCFullYear();
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  return `${day} ${mon} ${year}, ${hh}:${mm} GMT`;
}

/**
 * Operator line: `int · v1.0.201 · c82fc12 · Build #43474661245 · 13 Nov 2026, 17:38 GMT`
 * Omits missing fields; never invents placeholders.
 */
export function formatDeployIdentity(info?: DeployIdentity | null): string {
  if (!info) return "";
  const parts: string[] = [];
  if (info.env?.trim()) parts.push(info.env.trim());
  if (info.version?.trim()) parts.push(info.version.trim());
  const sha = shortSha(info.git_sha);
  if (sha) parts.push(sha);
  if (info.build_id?.trim()) parts.push(`Build #${info.build_id.trim()}`);
  const when = formatDeployedAtGmt(info.deployed_at);
  if (when) parts.push(when);
  return parts.join(" · ");
}
