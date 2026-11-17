import { NextResponse } from "next/server";
import { unifiedSearch } from "@/lib/search";

export const dynamic = "force-dynamic";

/**
 * MVP cross-entity typeahead. Query `?q=` (min 2 chars); returns flat hits
 * with hrefs into jobs / agents / members / groups / insights.
 */
export async function GET(req: Request) {
  const { searchParams } = new URL(req.url);
  const q = (searchParams.get("q") ?? "").trim();
  if (q.length < 2) {
    return NextResponse.json({ query: q, results: [] });
  }
  try {
    const body = await unifiedSearch(q);
    return NextResponse.json(body, {
      headers: { "Cache-Control": "public, s-maxage=15, stale-while-revalidate=30" },
    });
  } catch {
    return NextResponse.json(
      { error: { message: "Search failed" } },
      { status: 502 },
    );
  }
}
