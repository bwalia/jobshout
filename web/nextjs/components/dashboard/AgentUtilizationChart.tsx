"use client";

import {
  ResponsiveContainer,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";

interface AgentUtilizationData {
  name: string;
  utilization: number;
}

type DateRange = "7d" | "30d" | "90d";

// Mock utilisation data varies slightly per date range to give a realistic feel
const MOCK_DATA: Record<DateRange, AgentUtilizationData[]> = {
  "7d": [
    { name: "CodeReviewer", utilization: 88 },
    { name: "QA Sentinel", utilization: 72 },
    { name: "SprintCoach", utilization: 65 },
    { name: "TypeScript Mentor", utilization: 91 },
    { name: "K8s Guardian", utilization: 54 },
  ],
  "30d": [
    { name: "CodeReviewer", utilization: 82 },
    { name: "QA Sentinel", utilization: 69 },
    { name: "SprintCoach", utilization: 61 },
    { name: "TypeScript Mentor", utilization: 87 },
    { name: "K8s Guardian", utilization: 50 },
  ],
  "90d": [
    { name: "CodeReviewer", utilization: 78 },
    { name: "QA Sentinel", utilization: 65 },
    { name: "SprintCoach", utilization: 58 },
    { name: "TypeScript Mentor", utilization: 83 },
    { name: "K8s Guardian", utilization: 47 },
  ],
};

interface AgentUtilizationChartProps {
  dateRange: DateRange;
}

interface TooltipPayloadItem {
  value: number;
  name: string;
}

interface CustomTooltipProps {
  active?: boolean;
  payload?: TooltipPayloadItem[];
  label?: string;
}

function CustomTooltip({ active, payload, label }: CustomTooltipProps) {
  if (!active || !payload?.length) return null;

  return (
    <div className="rounded-md border border-border bg-card px-3 py-2 shadow-md text-sm">
      <p className="font-medium">{label}</p>
      <p className="mt-0.5 text-muted-foreground">
        Utilisation:{" "}
        <span className="font-semibold text-foreground">{payload[0]?.value}%</span>
      </p>
    </div>
  );
}

export function AgentUtilizationChart({ dateRange }: AgentUtilizationChartProps) {
  const data = MOCK_DATA[dateRange];

  return (
    <ResponsiveContainer width="100%" height={260}>
      <BarChart data={data} margin={{ top: 4, right: 4, left: -16, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
        <XAxis
          dataKey="name"
          tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          domain={[0, 100]}
          tickFormatter={(value: number) => `${value}%`}
          tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip content={<CustomTooltip />} cursor={{ fill: "hsl(var(--accent))" }} />
        <Bar
          dataKey="utilization"
          fill="hsl(var(--primary))"
          radius={[4, 4, 0, 0]}
          maxBarSize={48}
        />
      </BarChart>
    </ResponsiveContainer>
  );
}
