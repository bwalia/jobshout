import { Bot, FolderKanban, ClipboardList, TrendingUp } from "lucide-react";

interface DashboardStats {
  totalAgents: number;
  activeProjects: number;
  tasksToday: number;
  avgPerformance: number | null;
}

interface StatCardProps {
  title: string;
  value: string | number;
  description: string;
  icon: React.ElementType;
  accentBorder: string;
  iconBg: string;
  iconColor: string;
}

function StatCard({
  title,
  value,
  description,
  icon: Icon,
  accentBorder,
  iconBg,
  iconColor,
}: StatCardProps) {
  return (
    <div
      className={`rounded-xl border border-border border-l-4 bg-card p-5 shadow-card transition-all duration-200 hover:-translate-y-0.5 hover:shadow-card-hover ${accentBorder}`}
    >
      <div className="flex items-center gap-4">
        <span
          className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-lg ${iconBg} ${iconColor}`}
          aria-hidden="true"
        >
          <Icon className="h-5 w-5" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          <p className="mt-0.5 text-2xl font-bold tracking-tight text-foreground">
            {value}
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
        </div>
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
      accentBorder: "border-l-violet-500",
      iconBg: "bg-violet-50 dark:bg-violet-900/30",
      iconColor: "text-violet-600 dark:text-violet-400",
    },
    {
      title: "Active Projects",
      value: activeProjects,
      description: "Projects currently running",
      icon: FolderKanban,
      accentBorder: "border-l-blue-500",
      iconBg: "bg-blue-50 dark:bg-blue-900/30",
      iconColor: "text-blue-600 dark:text-blue-400",
    },
    {
      title: "Tasks In Progress",
      value: tasksToday,
      description: "Tasks currently in progress",
      icon: ClipboardList,
      accentBorder: "border-l-amber-500",
      iconBg: "bg-amber-50 dark:bg-amber-900/30",
      iconColor: "text-amber-600 dark:text-amber-400",
    },
    {
      title: "Avg Performance",
      value: avgPerformance !== null ? `${avgPerformance}%` : "--",
      icon: TrendingUp,
      description: "Average score across active agents",
      accentBorder: "border-l-emerald-500",
      iconBg: "bg-emerald-50 dark:bg-emerald-900/30",
      iconColor: "text-emerald-600 dark:text-emerald-400",
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
