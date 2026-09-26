import type { MetadataRoute } from "next";
import { listInsights, siteUrl } from "@/lib/insights";
import { listApps } from "@/lib/showcase";

export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl();
  const fixed: MetadataRoute.Sitemap = ["", "/jobs", "/showcase", "/insights", "/newsletter", "/post-job"].map((p) => ({
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
    for (let offset = 0; offset < 500; offset += 50) {
      const page = await listApps({ limit: 50, offset }).catch(() => ({ data: [], total: 0 }));
      apps.push(...page.data);
      if (apps.length >= page.total || page.data.length === 0) break;
    }
    return [
      ...fixed,
      ...apps.map((a) => ({
        url: `${base}/showcase/${a.slug}`,
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
