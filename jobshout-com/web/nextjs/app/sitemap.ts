import type { MetadataRoute } from "next";
import { listInsights, siteUrl } from "@/lib/insights";

export const dynamic = "force-dynamic";

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl();
  const fixed: MetadataRoute.Sitemap = ["", "/jobs", "/insights", "/newsletter", "/post-job"].map((p) => ({
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
    return [
      ...fixed,
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
