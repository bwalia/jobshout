/** Download a blob from an authenticated API path (e.g. report.pdf). */
export async function downloadAuthenticatedPDF(
  path: string,
  fallbackName: string,
  getBlob: (path: string) => Promise<Blob>
): Promise<void> {
  const blob = await getBlob(path);
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = fallbackName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export function reportVersionLabel(opts: {
  report_seq?: number | null;
  app_version?: string | null;
  completed_at?: string | null;
  created_at?: string;
}): string {
  const parts: string[] = [];
  if (opts.report_seq && opts.report_seq > 0) {
    parts.push(`v${opts.report_seq}`);
  }
  if (opts.app_version) {
    parts.push(opts.app_version);
  }
  const when = opts.completed_at || opts.created_at;
  if (when) {
    parts.push(new Date(when).toLocaleString());
  }
  return parts.join(" · ") || "—";
}
