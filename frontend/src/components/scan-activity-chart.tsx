import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from "recharts";
import { useScanActivity } from "../api/hooks/use-scans";
import { Skeleton } from "./skeleton";

export function ScanActivityChart() {
  const { data, isLoading } = useScanActivity();

  const isEmpty =
    !isLoading &&
    data.every(
      (d) => d.completed === 0 && d.failed === 0 && d.running === 0,
    );

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      <h3 className="text-xs font-medium text-text-primary mb-4">
        Scan Activity (7 days)
      </h3>
      {isLoading ? (
        <Skeleton className="h-40 w-full rounded" />
      ) : isEmpty ? (
        <div className="h-40 flex items-center justify-center">
          <p className="text-xs text-text-muted">
            No scan data for the past 7 days.
          </p>
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={160}>
          <AreaChart
            data={data}
            margin={{ top: 4, right: 4, left: -20, bottom: 0 }}
          >
            <defs>
              <linearGradient id="colorCompleted" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#22c55e" stopOpacity={0.3} />
                <stop offset="95%" stopColor="#22c55e" stopOpacity={0} />
              </linearGradient>
              <linearGradient id="colorFailed" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#ef4444" stopOpacity={0.3} />
                <stop offset="95%" stopColor="#ef4444" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid
              strokeDasharray="3 3"
              stroke="rgba(255,255,255,0.06)"
            />
            <XAxis
              dataKey="date"
              tick={{
                fontSize: 10,
                fill: "var(--color-text-muted, #6b7280)",
              }}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              allowDecimals={false}
              tick={{
                fontSize: 10,
                fill: "var(--color-text-muted, #6b7280)",
              }}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: "var(--color-surface-raised, #1e293b)",
                border: "1px solid var(--color-border, #334155)",
                borderRadius: 6,
                fontSize: 11,
              }}
              labelStyle={{
                color: "var(--color-text-primary, #f1f5f9)",
                marginBottom: 4,
              }}
              itemStyle={{ color: "var(--color-text-secondary, #94a3b8)" }}
            />
            <Area
              type="monotone"
              dataKey="completed"
              name="Completed"
              stroke="#22c55e"
              fill="url(#colorCompleted)"
              strokeWidth={1.5}
            />
            <Area
              type="monotone"
              dataKey="failed"
              name="Failed"
              stroke="#ef4444"
              fill="url(#colorFailed)"
              strokeWidth={1.5}
            />
          </AreaChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}
