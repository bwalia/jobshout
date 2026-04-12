import Link from "next/link";
import { AgentStatusBadge } from "@/components/agent/AgentStatusBadge";
import type { Agent } from "@/lib/types/agent";

interface AgentCardProps {
  agent: Agent;
  currentTask?: string;
}

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

function getInitials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join("");
}

function performanceColour(score: number): string {
  if (score >= 80) return "text-emerald-600 dark:text-emerald-400";
  if (score >= 50) return "text-amber-600 dark:text-amber-400";
  return "text-red-600 dark:text-red-400";
}

function performanceBgColour(score: number): string {
  if (score >= 80) return "bg-emerald-500";
  if (score >= 50) return "bg-amber-500";
  return "bg-red-500";
}

export function AgentCard({ agent, currentTask }: AgentCardProps) {
  const avatarColour = getAvatarColour(agent.name);
  const initials = getInitials(agent.name);

  return (
    <Link
      href={`/agents/${agent.id}`}
      className="group flex flex-col gap-4 rounded-xl border border-border bg-card p-5 shadow-card transition-all duration-200 hover:-translate-y-0.5 hover:shadow-card-hover"
    >
      {/* Top row: avatar + status */}
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          {agent.avatar_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={agent.avatar_url}
              alt={`${agent.name} avatar`}
              className="h-10 w-10 rounded-lg object-cover"
            />
          ) : (
            <span
              className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-sm font-semibold text-white ${avatarColour}`}
              aria-label={`${agent.name} avatar`}
            >
              {initials}
            </span>
          )}

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
      <div className="h-1.5 w-full rounded-full bg-secondary overflow-hidden">
        <div
          className={`h-full rounded-full transition-all ${performanceBgColour(agent.performance_score)}`}
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
