import { Bot, FolderKanban, ClipboardList, TrendingUp } from "lucide-react";

interface DashboardStats {
  totalAgents: number;
  activeProjects: number;
  tasksToday: number;
  /** Nullable when there are no agents yet */
  avgPerformance: number | null;
}

interface StatCardProps {
  title: string;
  value: string | number;
  description: string;
  icon: React.ElementType;
  /** Optional extra classes for the icon wrapper background */
  iconBgClass?: string;
}

function StatCard({
  title,
  value,
  description,
  icon: Icon,
  iconBgClass = "bg-primary/10 text-primary",
}: StatCardProps) {
  return (
    <div className="rounded-lg border border-border bg-card p-5 shadow-sm">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          <p className="mt-1 text-2xl font-bold tracking-tight text-foreground">
            {value}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">{description}</p>
        </div>
        <span
          className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md ${iconBgClass}`}
          aria-hidden="true"
        >
          <Icon className="h-5 w-5" />
        </span>
      </div>
    </div>
  );
}

interface StatsCardsProps {
  stats: DashboardStats;
}

export function StatsCards({ stats }: StatsCardsProps) {
  const { totalAgents, activeProjects, tasksToday, avgPerformance } = stats;

  const cards: StatCardProps[] = [
    {
      title: "Total Agents",
      value: totalAgents,
      description: "Agents in your organisation",
      icon: Bot,
      iconBgClass: "bg-violet-500/10 text-violet-500",
    },
    {
      title: "Active Projects",
      value: activeProjects,
      description: "Projects currently running",
      icon: FolderKanban,
      iconBgClass: "bg-blue-500/10 text-blue-500",
    },
    {
      title: "Tasks In Progress",
      value: tasksToday,
      description: "Tasks currently in progress",
      icon: ClipboardList,
      iconBgClass: "bg-amber-500/10 text-amber-500",
    },
    {
      title: "Avg Performance",
      value:
        avgPerformance !== null ? `${avgPerformance}%` : "—",
      description: "Average score across active agents",
      icon: TrendingUp,
      iconBgClass: "bg-emerald-500/10 text-emerald-500",
    },
  ];

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      {cards.map((card) => (
        <StatCard key={card.title} {...card} />
      ))}
    </div>
  );
}
