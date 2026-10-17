/**
 * OpsAPI CMS webhook receiver: refreshes Insights when someone presses publish /
 * "Website update" in OpsAPI. Lives outside /api/ because the gateway sends
 * /api/* to the Rust API, not to this app.
 *
 * OpsAPI signs the raw body: X-Opsapi-Signature-256: sha256=HMAC_SHA256(secret, body).
 * The secret is OPSAPI_WEBHOOK_SECRET (copied from the webhook's create response).
 */

import crypto from "node:crypto";
import { revalidatePath, revalidateTag } from "next/cache";
import { OPSAPI_TAG } from "@/lib/opsapi";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const MAX_AGE_MS = 5 * 60 * 1000;

function signatureMatches(raw: string, given: string, secret: string): boolean {
  const expected = "sha256=" + crypto.createHmac("sha256", secret).update(raw).digest("hex");
  const a = Buffer.from(given);
  const b = Buffer.from(expected);
  return a.length === b.length && crypto.timingSafeEqual(a, b);
}

export async function POST(req: Request) {
  const secret = process.env.OPSAPI_WEBHOOK_SECRET;
  if (!secret) return new Response("webhook not configured", { status: 503 });

  const raw = await req.text();
  if (!signatureMatches(raw, req.headers.get("x-opsapi-signature-256") ?? "", secret)) {
    return new Response("bad signature", { status: 401 });
  }

  let payload: { event?: string; timestamp?: string; data?: { slug?: string } };
  try {
    payload = JSON.parse(raw);
  } catch {
    return new Response("invalid JSON", { status: 400 });
  }
  const sent = Date.parse(payload.timestamp ?? "");
  if (!Number.isFinite(sent) || Math.abs(Date.now() - sent) > MAX_AGE_MS) {
    return new Response("stale request", { status: 401 });
  }

  revalidateTag(OPSAPI_TAG);
  revalidatePath("/insights");
  const slug = payload.data?.slug;
  if (slug && /^[a-z0-9-]+$/i.test(slug)) revalidatePath(`/insights/${slug}`);

  return Response.json({ ok: true, event: payload.event ?? null });
}
