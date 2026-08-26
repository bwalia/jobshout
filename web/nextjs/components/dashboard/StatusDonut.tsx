"use client";

import { ResponsiveContainer, PieChart, Pie, Cell, Tooltip } from "recharts";

export interface StatusSlice {
  name: string;
  value: number;
  color: string;
}

function DonutTooltip({
  active,
  payload,
}: {
  active?: boolean;
  payload?: { name: string; value: number }[];
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2 text-sm shadow-card-hover">
      <span className="font-medium">{payload[0].name}</span>{" "}
      <span className="text-muted-foreground">{payload[0].value}</span>
    </div>
  );
}

export function StatusDonut({
  slices,
  total,
}: {
  slices: StatusSlice[];
  total: number;
}) {
  const visible = slices.filter((s) => s.value > 0);

  return (
    <div>
      <div className="relative h-[180px]">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Tooltip content={<DonutTooltip />} />
            <Pie
              data={visible.length ? visible : [{ name: "None", value: 1, color: "hsl(var(--border))" }]}
              dataKey="value"
              nameKey="name"
              innerRadius={58}
              outerRadius={80}
              paddingAngle={visible.length > 1 ? 2 : 0}
              strokeWidth={0}
            >
              {(visible.length
                ? visible
                : [{ name: "None", value: 1, color: "hsl(var(--border))" }]
              ).map((s) => (
                <Cell key={s.name} fill={s.color} />
              ))}
            </Pie>
          </PieChart>
        </ResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-2xl font-semibold tracking-tight">{total}</span>
          <span className="text-xs text-muted-foreground">tasks</span>
        </div>
      </div>
      <ul className="mt-4 space-y-2">
        {slices.map((s) => (
          <li key={s.name} className="flex items-center gap-2 text-sm">
            <span
              className="h-2.5 w-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: s.color }}
            />
            <span className="flex-1 text-muted-foreground">{s.name}</span>
            <span className="font-mono text-xs font-medium text-foreground">
              {s.value}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
