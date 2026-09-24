import { fetchFeed } from "@/lib/insights";

// Rendered per request (the fetch itself is cached): the image build has no API to prerender from.
export const dynamic = "force-dynamic";

export async function GET() {
  const res = await fetchFeed("podcast");
  if (!res.ok) return new Response("Feed unavailable", { status: 502 });
  return new Response(await res.text(), {
    headers: {
      "content-type": "application/rss+xml; charset=utf-8",
      "cache-control": "public, max-age=300",
    },
  });
}
