import type { MetadataRoute } from "next";
import { listInsights, siteUrl } from "@/lib/insights";
import { entryHref, listApps } from "@/lib/showcase";

export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl();
  const fixed: MetadataRoute.Sitemap = ["", "/jobs", "/showcase", "/agents", "/insights", "/newsletter", "/post-job"].map((p) => ({
    url: `${base}${p}`,
    changeFrequency: "daily",
  }));
  try {
    const items = [];
    // 50 per page is the API ceiling; a few pages covers the archive for now.
    for (let offset = 0; offset < 500; offset += 50) {
      const page = await listInsights({ limit: 50, offset });
      items.push(...page.data);
      if (items.length >= page.total || page.data.length === 0) break;
    }
    const apps = [];
    for (const kind of ["app", "agent", "team"] as const) {
      let seen = 0;
      for (let offset = 0; offset < 500; offset += 50) {
        const page = await listApps({ kind, limit: 50, offset }).catch(() => ({ data: [], total: 0 }));
        apps.push(...page.data);
        seen += page.data.length;
        if (seen >= page.total || page.data.length === 0) break;
      }
    }
    return [
      ...fixed,
      ...apps.map((a) => ({
        url: `${base}${entryHref(a)}`,
        lastModified: a.updated_at,
        changeFrequency: "weekly" as const,
      })),
      ...items.map((i) => ({
        url: `${base}/insights/${i.slug}`,
        lastModified: i.updated_at,
        changeFrequency: "weekly" as const,
      })),
    ];
  } catch {
    return fixed;
  }
}
