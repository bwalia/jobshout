"use client";

import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from "recharts";

interface TaskCompletionDataPoint {
  day: string;
  tasks: number;
}

type DateRange = "7d" | "30d" | "90d";

// Mock task-completion data for each date range.
// 7d shows the last 7 days; 30d and 90d are weekly/bi-weekly rollups for readability.
const MOCK_DATA: Record<DateRange, TaskCompletionDataPoint[]> = {
  "7d": [
    { day: "Mon", tasks: 18 },
    { day: "Tue", tasks: 24 },
    { day: "Wed", tasks: 21 },
    { day: "Thu", tasks: 29 },
    { day: "Fri", tasks: 22 },
    { day: "Sat", tasks: 14 },
    { day: "Sun", tasks: 14 },
  ],
  "30d": [
    { day: "Week 1", tasks: 112 },
    { day: "Week 2", tasks: 138 },
    { day: "Week 3", tasks: 155 },
    { day: "Week 4", tasks: 184 },
  ],
  "90d": [
    { day: "Jan W1-2", tasks: 198 },
    { day: "Jan W3-4", tasks: 231 },
    { day: "Feb W1-2", tasks: 245 },
    { day: "Feb W3-4", tasks: 278 },
    { day: "Mar W1-2", tasks: 460 },
    { day: "Mar W3-4", tasks: 460 },
  ],
};

interface TaskCompletionChartProps {
  dateRange: DateRange;
}

interface TooltipPayloadItem {
  value: number;
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
        Tasks:{" "}
        <span className="font-semibold text-foreground">{payload[0]?.value}</span>
      </p>
    </div>
  );
}

export function TaskCompletionChart({ dateRange }: TaskCompletionChartProps) {
  const data = MOCK_DATA[dateRange];

  return (
    <ResponsiveContainer width="100%" height={260}>
      <LineChart data={data} margin={{ top: 4, right: 4, left: -16, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
        <XAxis
          dataKey="day"
          tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          tick={{ fontSize: 12, fill: "hsl(var(--muted-foreground))" }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip content={<CustomTooltip />} />
        <Line
          type="monotone"
          dataKey="tasks"
          stroke="hsl(var(--primary))"
          strokeWidth={2}
          dot={{ r: 4, fill: "hsl(var(--primary))", strokeWidth: 0 }}
          activeDot={{ r: 6 }}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}
