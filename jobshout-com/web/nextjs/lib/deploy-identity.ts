/** Deploy identity for the site footer. Runtime Helm + image-baked env. */

export type DeployIdentity = {
  version?: string;
  env?: string;
  gitSha?: string;
  buildId?: string;
  deployedAt?: string;
};

/** Read identity from process env (SSR). Empty when unset — no fakes. */
export function readDeployIdentity(): DeployIdentity {
  return {
    version: trim(process.env.APP_VERSION),
    env: trim(process.env.APP_ENV),
    gitSha: trim(process.env.APP_GIT_SHA),
    buildId: trim(process.env.APP_BUILD_ID),
    deployedAt: trim(process.env.APP_DEPLOYED_AT),
  };
}

function trim(v: string | undefined): string | undefined {
  const s = v?.trim();
  return s || undefined;
}

function shortSha(sha?: string): string | undefined {
  if (!sha) return undefined;
  return sha.length > 7 ? sha.slice(0, 7) : sha;
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
  if (info.env) parts.push(info.env);
  if (info.version && info.version !== "latest") parts.push(info.version);
  const sha = shortSha(info.gitSha);
  if (sha) parts.push(sha);
  if (info.buildId) parts.push(`Build #${info.buildId}`);
  const when = formatDeployedAtGmt(info.deployedAt);
  if (when) parts.push(when);
  return parts.join(" · ");
}
