import Link from "next/link";
import { AgentStatusBadge } from "@/components/agent/AgentStatusBadge";
import type { Agent } from "@/lib/types/agent";

interface AgentCardProps {
  agent: Agent;
  /** Optional: title of the task the agent is currently working on */
  currentTask?: string;
}

// A palette of background colours cycled by the first character of the agent
// name so that each agent gets a consistent, visually distinct avatar colour.
const AVATAR_COLOURS = [
  "bg-violet-600",
  "bg-blue-600",
  "bg-emerald-600",
  "bg-amber-600",
  "bg-rose-600",
  "bg-cyan-600",
  "bg-indigo-600",
  "bg-pink-600",
];

function getAvatarColour(name: string): string {
  const index = name.charCodeAt(0) % AVATAR_COLOURS.length;
  return AVATAR_COLOURS[index];
}

/** Derives up to two initials from an agent name. e.g. "Content Writer" → "CW" */
function getInitials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join("");
}

/** Clamps a performance score (0–100) to a coloured text label. */
function performanceColour(score: number): string {
  if (score >= 80) return "text-emerald-500";
  if (score >= 50) return "text-amber-500";
  return "text-red-500";
}

export function AgentCard({ agent, currentTask }: AgentCardProps) {
  const avatarColour = getAvatarColour(agent.name);
  const initials = getInitials(agent.name);

  return (
    <Link
      href={`/agents/${agent.id}`}
      className="group flex flex-col gap-4 rounded-lg border border-border bg-card p-4 shadow-sm transition-colors hover:border-primary/50 hover:bg-accent/30"
    >
      {/* Top row: avatar + status */}
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          {/* Avatar – image if provided, initials circle otherwise */}
          {agent.avatar_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={agent.avatar_url}
              alt={`${agent.name} avatar`}
              className="h-10 w-10 rounded-full object-cover"
            />
          ) : (
            <span
              className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full text-sm font-semibold text-white ${avatarColour}`}
              aria-label={`${agent.name} avatar`}
            >
              {initials}
            </span>
          )}

          {/* Name + role */}
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-foreground group-hover:text-primary transition-colors">
              {agent.name}
            </p>
            <p className="truncate text-xs text-muted-foreground">
              {agent.role}
            </p>
          </div>
        </div>

        <AgentStatusBadge status={agent.status} />
      </div>

      {/* Performance score */}
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">Performance</span>
        <span
          className={`text-sm font-semibold ${performanceColour(agent.performance_score)}`}
        >
          {agent.performance_score}%
        </span>
      </div>

      {/* Progress bar */}
      <div className="h-1.5 w-full rounded-full bg-muted overflow-hidden">
        <div
          className={`h-full rounded-full transition-all ${performanceColour(agent.performance_score).replace("text-", "bg-")}`}
          style={{ width: `${agent.performance_score}%` }}
          aria-valuenow={agent.performance_score}
          aria-valuemin={0}
          aria-valuemax={100}
          role="progressbar"
        />
      </div>

      {/* Current task (if provided) */}
      {currentTask && (
        <p className="truncate text-xs text-muted-foreground">
          <span className="font-medium text-foreground">Working on:</span>{" "}
          {currentTask}
        </p>
      )}
    </Link>
  );
}
