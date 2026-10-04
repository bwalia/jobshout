"use client";

import { useEffect } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Loader2 } from "lucide-react";
import { useAgents } from "@/lib/hooks/useAgents";
import { useAgentSchemas } from "@/lib/hooks/useAgentSchemas";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Keys that identify where to go; everything else rides along. */
const ROUTING_KEYS = new Set(["agent", "project", "view"]);

function rest(params: URLSearchParams): URLSearchParams {
  const out = new URLSearchParams();
  params.forEach((v, k) => {
    if (!ROUTING_KEYS.has(k)) out.set(k, v);
  });
  return out;
}

function withQuery(path: string, q: URLSearchParams): string {
  const s = q.toString();
  if (!s) return path;
  return `${path}${path.includes("?") ? "&" : "?"}${s}`;
}

/**
 * Task Board and Task Manager were folded into Tasks and the agent profile.
 * Old links (bookmarks, chat cards, the Gmail and LinkedIn OAuth returns) land
 * here and are sent on with every other query parameter kept:
 *   ?agent=<specialist slug>  → that agent's Workspace tab
 *   ?agent=<agent id>         → Tasks filtered to that agent
 *   ?project=<id>             → Tasks for that project
 *   ?view=agents (Task Board) → Tasks grouped by agent
 */
export function LegacyTaskRedirect({ from }: { from: "task-board" | "task-manager" }) {
  const router = useRouter();
  const params = useSearchParams();
  const agentParam = params.get("agent");
  const isSlug = Boolean(agentParam && !UUID.test(agentParam));
  const { data: catalog, isError: catalogError } = useAgentSchemas();
  const { data: agentsResp, isError: agentsError } = useAgents({ per_page: 100 });

  useEffect(() => {
    const carry = rest(params);

    if (isSlug) {
      if (!catalog && !catalogError) return;
      if (!agentsResp && !agentsError) return;
      const wire = catalog?.find((w) => w.tab_slug === agentParam || w.builtin === agentParam);
      const agent = agentsResp?.data?.find((a) => a.metadata?.builtin === wire?.builtin);
      router.replace(
        agent ? withQuery(`/panel/agents/${agent.id}?tab=workspace`, carry) : "/panel/agents"
      );
      return;
    }

    const q = new URLSearchParams(carry);
    const project = params.get("project");
    if (project) q.set("project", project);
    if (agentParam) q.set("agent", agentParam);
    if (from === "task-board" && params.get("view") === "agents") q.set("group", "agent");
    router.replace(withQuery("/panel/tasks", q));
  }, [isSlug, catalog, catalogError, agentsResp, agentsError, agentParam, params, from, router]);

  return (
    <div className="flex h-40 items-center justify-center gap-2 text-sm text-muted-foreground">
      <Loader2 className="h-4 w-4 animate-spin" />
      {from === "task-manager" ? "Task Manager moved — taking you there…" : "Task Board moved to Tasks…"}
    </div>
  );
}
