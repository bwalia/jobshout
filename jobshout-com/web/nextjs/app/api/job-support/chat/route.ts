import { NextResponse, type NextRequest } from "next/server";
import { newSession, runJobSupportTurn } from "@/lib/job-support/agent";
import { encodeSse, type JobSupportEvent } from "@/lib/job-support/events";
import { nebuladbEnabled } from "@/lib/job-support/nebuladb";
import type { SessionState } from "@/lib/job-support/types";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
export const maxDuration = 300;

type Body = {
  message?: string;
  session?: Partial<SessionState> | null;
  history?: { role: "user" | "assistant"; content: string }[];
};

/** Lets the dock show offline state up front. */
export async function GET() {
  return NextResponse.json(
    { enabled: nebuladbEnabled() },
    { headers: { "Cache-Control": "no-store" } },
  );
}

export async function POST(req: NextRequest) {
  if (!nebuladbEnabled()) {
    return NextResponse.json(
      {
        error: {
          code: "JOB_SUPPORT_OFFLINE",
          message: "Job Support is offline right now — browse jobs or post a role and our team will help.",
        },
      },
      { status: 503 },
    );
  }

  let body: Body;
  try {
    body = (await req.json()) as Body;
  } catch {
    return NextResponse.json(
      { error: { code: "BAD_JSON", message: "Invalid JSON body" } },
      { status: 400 },
    );
  }

  const message = (body.message ?? "").trim();
  if (!message || message.length > 20_000) {
    return NextResponse.json(
      { error: { code: "VALIDATION", message: "message is required (max 20k)" } },
      { status: 400 },
    );
  }

  const base = newSession();
  const session: SessionState = {
    ...base,
    ...(body.session ?? {}),
    id: body.session?.id || base.id,
  };
  const history = Array.isArray(body.history) ? body.history.slice(-12) : [];

  const stream = new ReadableStream({
    async start(controller) {
      const enc = new TextEncoder();
      const send = (e: JobSupportEvent) => {
        controller.enqueue(enc.encode(encodeSse(e)));
      };
      send({ type: "session", session_id: session.id });
      try {
        const next = await runJobSupportTurn({
          session,
          message,
          history,
          emit: send,
        });
        send({
          type: "session_state",
          state: {
            id: next.id,
            role: next.role,
            docId: next.docId,
            contactSaved: next.contactSaved,
            brief: next.brief,
            cv: next.cv,
            name: next.name,
            email: next.email,
            phone: next.phone,
            company: next.company,
          },
        });
      } catch (err) {
        send({
          type: "error",
          message: err instanceof Error ? err.message : "Unexpected error",
          code: "TURN_FAILED",
        });
        send({ type: "done", role: session.role });
      } finally {
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "content-type": "text/event-stream; charset=utf-8",
      "cache-control": "no-store, no-transform",
      connection: "keep-alive",
    },
  });
}
