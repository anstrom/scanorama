import { useState } from "react";
import {
  Server,
  Activity,
  Settings,
  FileText,
  Database,
  Clock,
  GitCommit,
  Package,
  AlertCircle,
  Save,
  AlertTriangle,
  Link2,
  Plus,
  Trash2,
  FlaskConical,
  ChevronDown,
  ChevronRight,
  CheckCircle,
  XCircle,
} from "lucide-react";
import {
  useAdminStatus,
  useWorkers,
  useVersion,
} from "../api/hooks/use-system";
import { useSettings, useUpdateSetting } from "../api/hooks/use-dashboard";
import { StatusBadge, Skeleton } from "../components";
import { cn, formatRelativeTime } from "../lib/utils";
import { LogViewer } from "../components/log-viewer";
import { Button } from "../components/button";
import { useToast } from "../components/toast-provider";
import {
  useWebhooks,
  useCreateWebhook,
  useUpdateWebhook,
  useDeleteWebhook,
  useTestWebhook,
  useDeliveryLogs,
  type WebhookEndpoint,
  type WebhookDeliveryLog,
} from "../api/hooks/use-webhooks";

// ── Helpers ────────────────────────────────────────────────────────────────────

function healthColorClass(status: string): string {
  const s = status.toLowerCase();
  if (s === "healthy" || s === "active") return "bg-success/15 text-success";
  if (s === "degraded") return "bg-warning/15 text-warning";
  if (s === "unhealthy" || s === "error") return "bg-danger/15 text-danger";
  return "bg-text-muted/15 text-text-muted";
}

function HealthBadge({ status }: { status: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center px-2 py-0.5 rounded text-xs font-medium",
        healthColorClass(status),
      )}
    >
      {status}
    </span>
  );
}

function InfoRow({
  icon: Icon,
  label,
  value,
  mono = false,
}: {
  icon: React.ElementType;
  label: string;
  value: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="flex items-start gap-2">
      <Icon className="h-3.5 w-3.5 text-text-muted shrink-0 mt-0.5" />
      <div className="flex flex-col gap-0.5 min-w-0">
        <span className="text-[10px] uppercase tracking-wide text-text-muted">
          {label}
        </span>
        <span
          className={cn(
            "text-xs text-text-primary truncate",
            mono && "font-mono",
          )}
        >
          {value ?? "—"}
        </span>
      </div>
    </div>
  );
}

// ── Section 1 — System Status ──────────────────────────────────────────────────

function SystemStatusCard() {
  const {
    data: adminStatus,
    isLoading: statusLoading,
    error: statusError,
  } = useAdminStatus();
  const { data: version, isLoading: versionLoading } = useVersion();

  // The actual shape returned by the Go handler (fields beyond the generated types)
  const status = adminStatus as Record<string, unknown> | undefined;
  const serverInfo = (status?.server_info ?? {}) as Record<string, unknown>;
  const ver = version as Record<string, unknown> | undefined;

  const healthStatus =
    (status?.admin_status as string | undefined) ?? "unknown";
  const serverAddress = (serverInfo.address as string | undefined) ?? "—";
  const dbConnected = (status as Record<string, unknown> | undefined)
    ?.database as { connected?: boolean } | undefined;

  const versionStr = (ver?.version as string | undefined) ?? "—";
  const commitStr = (ver?.commit as string | undefined) ?? null;
  const buildTimeStr = (ver?.build_time as string | undefined) ?? null;

  if (statusLoading || versionLoading) {
    return (
      <div className="bg-surface rounded-lg border border-border p-4">
        <div className="flex items-center gap-2 mb-4">
          <Server className="h-4 w-4 text-text-muted" />
          <Skeleton className="h-3.5 w-28" />
        </div>
        <div className="space-y-3">
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-3 w-48" />
          <div className="h-px bg-border my-3" />
          <div className="grid grid-cols-3 gap-4">
            <Skeleton className="h-8 rounded" />
            <Skeleton className="h-8 rounded" />
            <Skeleton className="h-8 rounded" />
          </div>
        </div>
      </div>
    );
  }

  if (statusError) {
    return (
      <div className="bg-surface rounded-lg border border-border p-4">
        <div className="flex items-center gap-2 text-danger text-xs">
          <AlertCircle className="h-3.5 w-3.5 shrink-0" />
          <span>
            Failed to load system status. Check your connection or API key.
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-surface rounded-lg border border-border p-4">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <Server className="h-4 w-4 text-text-muted" />
          <span className="text-xs font-medium text-text-primary">
            System Status
          </span>
        </div>
        <HealthBadge status={healthStatus} />
      </div>

      {/* Status grid */}
      <div className="grid grid-cols-2 md:grid-cols-3 gap-x-6 gap-y-4 mb-4">
        <InfoRow icon={Server} label="API address" value={serverAddress} mono />

        {dbConnected !== undefined ? (
          <InfoRow
            icon={Database}
            label="Database"
            value={
              <StatusBadge
                status={dbConnected.connected ? "connected" : "disconnected"}
              />
            }
          />
        ) : (
          <InfoRow icon={Database} label="Database" value="—" />
        )}

        <InfoRow
          icon={Clock}
          label="Timestamp"
          value={
            status?.timestamp
              ? formatRelativeTime(status.timestamp as string)
              : "—"
          }
        />
      </div>

      <div className="h-px bg-border mb-4" />

      {/* Build info */}
      <div className="grid grid-cols-3 gap-x-4 gap-y-3">
        <div className="flex flex-col gap-1 min-w-0">
          <div className="flex items-center gap-1 text-text-muted">
            <Package className="h-3 w-3 shrink-0" />
            <span className="text-[10px] uppercase tracking-wide">Version</span>
          </div>
          <span className="text-xs font-mono text-text-primary truncate">
            {versionStr}
          </span>
        </div>

        <div className="flex flex-col gap-1 min-w-0">
          <div className="flex items-center gap-1 text-text-muted">
            <GitCommit className="h-3 w-3 shrink-0" />
            <span className="text-[10px] uppercase tracking-wide">Commit</span>
          </div>
          <span
            className={cn(
              "text-xs font-mono truncate",
              commitStr && commitStr !== "none"
                ? "text-text-primary"
                : "text-text-muted",
            )}
          >
            {commitStr && commitStr !== "none" ? commitStr : "—"}
          </span>
        </div>

        <div className="flex flex-col gap-1 min-w-0">
          <div className="flex items-center gap-1 text-text-muted">
            <Clock className="h-3 w-3 shrink-0" />
            <span className="text-[10px] uppercase tracking-wide">Built</span>
          </div>
          <span
            className={cn(
              "text-xs truncate",
              buildTimeStr && buildTimeStr !== "unknown"
                ? "text-text-secondary"
                : "text-text-muted",
            )}
          >
            {buildTimeStr && buildTimeStr !== "unknown"
              ? (() => {
                  try {
                    return new Date(buildTimeStr).toLocaleString(undefined, {
                      year: "numeric",
                      month: "short",
                      day: "numeric",
                      hour: "2-digit",
                      minute: "2-digit",
                    });
                  } catch {
                    return buildTimeStr;
                  }
                })()
              : "—"}
          </span>
        </div>
      </div>
    </div>
  );
}

// ── Section 2 — Workers table ──────────────────────────────────────────────────

interface WorkerInfo {
  id: string;
  status: string;
  start_time?: string;
  current_job?: {
    type?: string;
    target?: string;
  } | null;
}

function workerTaskDescription(worker: WorkerInfo): string {
  if (worker.current_job) {
    const { type, target } = worker.current_job;
    if (type && target) return `${type}: ${target}`;
    if (type) return type;
  }
  return worker.status === "idle" ? "Idle" : "—";
}

function WorkersCard() {
  const { data: workersRaw, isLoading } = useWorkers();

  // The endpoint isn't in the generated types yet; handle shape at runtime.
  const payload = workersRaw as Record<string, unknown> | undefined;
  const workers: WorkerInfo[] = Array.isArray(payload?.workers)
    ? (payload!.workers as WorkerInfo[])
    : Array.isArray(payload?.data)
      ? (payload!.data as WorkerInfo[])
      : [];

  return (
    <div className="bg-surface rounded-lg border border-border overflow-hidden">
      {/* Card header */}
      <div className="px-4 py-3 border-b border-border flex items-center gap-2">
        <Activity className="h-4 w-4 text-text-muted" />
        <span className="text-xs font-medium text-text-primary">Workers</span>
        {!isLoading && workers.length > 0 && (
          <span className="ml-auto text-[10px] text-text-muted">
            {workers.length} worker{workers.length !== 1 ? "s" : ""}
          </span>
        )}
      </div>

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border">
              {["ID", "Status", "Task", "Started"].map((col) => (
                <th
                  key={col}
                  className="text-left px-4 py-2 text-[10px] uppercase tracking-wide text-text-muted font-medium"
                >
                  {col}
                </th>
              ))}
            </tr>
          </thead>

          <tbody>
            {/* Loading skeleton */}
            {isLoading &&
              Array.from({ length: 3 }).map((_, i) => (
                <tr key={i} className="border-b border-border last:border-0">
                  <td className="px-4 py-3">
                    <Skeleton className="h-3 w-20" />
                  </td>
                  <td className="px-4 py-3">
                    <Skeleton className="h-3 w-14" />
                  </td>
                  <td className="px-4 py-3">
                    <Skeleton className="h-3 w-32" />
                  </td>
                  <td className="px-4 py-3">
                    <Skeleton className="h-3 w-16" />
                  </td>
                </tr>
              ))}

            {/* Workers rows */}
            {!isLoading &&
              workers.map((worker) => (
                <tr
                  key={worker.id}
                  className="border-b border-border last:border-0 hover:bg-surface-raised/50 transition-colors"
                >
                  <td className="px-4 py-3 font-mono text-text-secondary">
                    <span title={worker.id}>
                      {worker.id.length > 12
                        ? `${worker.id.slice(0, 12)}…`
                        : worker.id}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={worker.status} />
                  </td>
                  <td className="px-4 py-3 text-text-secondary">
                    {workerTaskDescription(worker)}
                  </td>
                  <td className="px-4 py-3 text-text-muted whitespace-nowrap">
                    {worker.start_time
                      ? formatRelativeTime(worker.start_time)
                      : "—"}
                  </td>
                </tr>
              ))}

            {/* Empty state */}
            {!isLoading && workers.length === 0 && (
              <tr>
                <td
                  colSpan={4}
                  className="px-4 py-8 text-center text-text-muted"
                >
                  No workers running
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ── Section 3 — Settings editor ───────────────────────────────────────────────

const SETTING_SECTIONS: Record<string, { label: string; keys: string[] }> = {
  scanning: {
    label: "Scanning",
    keys: ["scan.default_timing", "scan.max_concurrent"],
  },
  discovery: {
    label: "Discovery",
    keys: ["discovery.ping_timeout_ms", "discovery.methods"],
  },
  retention: {
    label: "Data Retention",
    keys: ["retention.auto_purge_days", "retention.max_scan_history"],
  },
  notifications: {
    label: "Notifications",
    keys: [
      "notifications.scan_complete",
      "notifications.host_down",
      "notifications.new_host",
    ],
  },
};

// Keys that require server restart to take effect
const RESTART_REQUIRED = new Set<string>([]);

function SettingField({
  setting,
}: {
  setting: { key: string; value: string; type: string; description: string };
}) {
  const { toast } = useToast();
  const [localValue, setLocalValue] = useState(setting.value);
  const [dirty, setDirty] = useState(false);
  const { mutateAsync: updateSetting, isPending } = useUpdateSetting();

  const needsRestart = RESTART_REQUIRED.has(setting.key);

  function handleChange(v: string) {
    setLocalValue(v);
    setDirty(v !== setting.value);
  }

  async function handleSave() {
    // Serialize value correctly
    let jsonValue: string;
    if (setting.type === "bool") {
      jsonValue = localValue === "true" ? "true" : "false";
    } else if (setting.type === "int") {
      const n = parseInt(localValue, 10);
      jsonValue = isNaN(n) ? setting.value : String(n);
    } else {
      jsonValue = localValue;
    }
    try {
      await updateSetting({ key: setting.key, value: jsonValue });
      setDirty(false);
      toast.success("Setting saved.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save setting.");
    }
  }

  const label = setting.key.split(".").pop()!.replace(/_/g, " ");

  return (
    <div className="flex items-start gap-3 py-2.5 border-b border-border/40 last:border-0">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5 mb-0.5">
          <span className="text-xs font-medium text-text-primary capitalize">
            {label}
          </span>
          {needsRestart && (
            <span
              title="Requires server restart"
              className="inline-flex items-center gap-0.5 text-[10px] text-warning"
            >
              <AlertTriangle className="h-3 w-3" />
              restart required
            </span>
          )}
        </div>
        {setting.description && (
          <p className="text-[11px] text-text-muted">{setting.description}</p>
        )}
      </div>
      <div className="flex items-center gap-2 shrink-0">
        {setting.type === "bool" ? (
          <select
            value={localValue}
            onChange={(e) => handleChange(e.target.value)}
            className={cn(
              "px-2 py-1 text-xs rounded border border-border",
              "bg-surface text-text-primary",
              "focus:outline-none focus:ring-1 focus:ring-border",
            )}
          >
            <option value="true">Enabled</option>
            <option value="false">Disabled</option>
          </select>
        ) : (
          <input
            type={setting.type === "int" ? "number" : "text"}
            value={localValue}
            onChange={(e) => handleChange(e.target.value)}
            className={cn(
              "px-2 py-1 text-xs rounded border border-border w-28",
              "bg-surface text-text-primary placeholder:text-text-muted",
              "focus:outline-none focus:ring-1 focus:ring-border",
            )}
          />
        )}
        <Button
          onClick={() => void handleSave()}
          loading={isPending}
          disabled={!dirty}
          icon={<Save className="h-3 w-3" />}
          className="text-xs h-7 px-2"
        >
          Save
        </Button>
      </div>
    </div>
  );
}

function SettingsCard() {
  const { data: settings = [], isLoading } = useSettings();
  const [activeTab, setActiveTab] = useState("scanning");
  const tabs = Object.entries(SETTING_SECTIONS);
  const activeSection = SETTING_SECTIONS[activeTab]!;
  const sectionSettings = settings.filter((s) =>
    activeSection.keys.includes(s.key),
  );

  return (
    <div className="bg-surface rounded-lg border border-border overflow-hidden">
      <div className="px-4 py-3 border-b border-border flex items-center gap-2">
        <Settings className="h-4 w-4 text-text-muted" />
        <span className="text-xs font-medium text-text-primary">
          Configuration
        </span>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-border overflow-x-auto">
        {tabs.map(([key, { label }]) => (
          <button
            key={key}
            type="button"
            onClick={() => setActiveTab(key)}
            className={cn(
              "px-4 py-2 text-xs whitespace-nowrap transition-colors",
              activeTab === key
                ? "border-b-2 border-accent text-accent font-medium"
                : "text-text-muted hover:text-text-primary",
            )}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="px-4 py-2">
        {isLoading ? (
          <div className="space-y-3 py-2">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex items-center justify-between">
                <Skeleton className="h-3 w-40" />
                <Skeleton className="h-6 w-24" />
              </div>
            ))}
          </div>
        ) : sectionSettings.length === 0 ? (
          <p className="text-xs text-text-muted py-4 text-center">
            No settings available. Settings are loaded from the database.
          </p>
        ) : (
          sectionSettings.map((s) => (
            <SettingField key={s.key} setting={s} />
          ))
        )}
      </div>
    </div>
  );
}

// ── Section 5 — Webhooks ──────────────────────────────────────────────────────

const WEBHOOK_EVENTS = [
  { value: "host.online", label: "Host Online" },
  { value: "host.offline", label: "Host Offline" },
  { value: "scan.started", label: "Scan Started" },
  { value: "scan.completed", label: "Scan Completed" },
  { value: "discovery.completed", label: "Discovery Completed" },
] as const;

function statusCodeClass(code: number | null): string {
  if (code === null) return "text-text-muted";
  if (code >= 200 && code < 300) return "text-success";
  return "text-danger";
}

function DeliveryLogsPanel({ endpointId }: { endpointId: string }) {
  const { data: logs = [], isLoading, isError } = useDeliveryLogs(endpointId);

  if (isError) {
    return (
      <div className="px-4 py-3">
        <p className="text-xs text-danger">Failed to load delivery logs.</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="p-3 space-y-2">
        {[1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-4 w-full" />
        ))}
      </div>
    );
  }

  if (logs.length === 0) {
    return (
      <div className="px-4 py-3 text-xs text-text-muted">
        No delivery logs yet.
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-border/40">
            {["Event", "Status", "Attempts", "Delivered", "Error"].map((h) => (
              <th
                key={h}
                className="text-left px-4 py-2 text-[10px] uppercase tracking-wide text-text-muted font-medium whitespace-nowrap"
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {(logs as WebhookDeliveryLog[]).map((log) => (
            <tr key={log.id} className="border-b border-border/20 last:border-0">
              <td className="px-4 py-2 font-mono text-text-secondary whitespace-nowrap">
                {log.event_type}
              </td>
              <td className={cn("px-4 py-2 font-mono whitespace-nowrap", statusCodeClass(log.status_code))}>
                {log.status_code ?? "—"}
              </td>
              <td className="px-4 py-2 text-text-muted">{log.attempt_count}</td>
              <td className="px-4 py-2 text-text-muted whitespace-nowrap">
                {log.delivered_at ? formatRelativeTime(log.delivered_at) : "—"}
              </td>
              <td className="px-4 py-2 text-text-muted truncate max-w-xs">
                {log.last_error ?? "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function WebhookRow({
  endpoint,
  expanded,
  onToggleExpand,
}: {
  endpoint: WebhookEndpoint;
  expanded: boolean;
  onToggleExpand: () => void;
}) {
  const { toast } = useToast();
  const { mutateAsync: updateWebhook, isPending: updatePending } = useUpdateWebhook();
  const { mutateAsync: deleteWebhook, isPending: deletePending } = useDeleteWebhook();
  const { mutateAsync: testWebhook, isPending: testPending } = useTestWebhook();

  async function handleToggleEnabled() {
    try {
      await updateWebhook({ id: endpoint.id, body: { enabled: !endpoint.enabled } });
    } catch {
      toast.error("Failed to update webhook.");
    }
  }

  async function handleDelete() {
    if (!confirm(`Delete webhook for ${endpoint.url}?`)) return;
    try {
      await deleteWebhook(endpoint.id);
      toast.success("Webhook deleted.");
    } catch {
      toast.error("Failed to delete webhook.");
    }
  }

  async function handleTest() {
    try {
      await testWebhook(endpoint.id);
      toast.success("Test delivery sent successfully.");
    } catch {
      toast.error("Test delivery failed. Check delivery logs for details.");
    }
  }

  return (
    <>
      <tr
        className="border-b border-border last:border-0 hover:bg-surface-raised/30 transition-colors"
      >
        <td className="px-4 py-3">
          <button
            type="button"
            onClick={onToggleExpand}
            className="flex items-center gap-1 text-text-primary hover:text-text-primary"
          >
            {expanded ? (
              <ChevronDown className="h-3 w-3 text-text-muted shrink-0" />
            ) : (
              <ChevronRight className="h-3 w-3 text-text-muted shrink-0" />
            )}
            <span className="font-mono text-xs truncate max-w-xs">{endpoint.url}</span>
          </button>
        </td>
        <td className="px-4 py-3">
          <div className="flex flex-wrap gap-1">
            {endpoint.events.map((e) => (
              <span
                key={e}
                className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-accent/10 text-accent"
              >
                {e}
              </span>
            ))}
          </div>
        </td>
        <td className="px-4 py-3">
          {endpoint.enabled ? (
            <span className="inline-flex items-center gap-1 text-success text-xs">
              <CheckCircle className="h-3 w-3" />
              Enabled
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 text-text-muted text-xs">
              <XCircle className="h-3 w-3" />
              Disabled
            </span>
          )}
        </td>
        <td className="px-4 py-3 text-xs text-text-muted whitespace-nowrap">
          {formatRelativeTime(endpoint.created_at)}
        </td>
        <td className="px-4 py-3">
          <div className="flex items-center gap-1">
            <Button
              onClick={() => void handleToggleEnabled()}
              loading={updatePending}
              className="text-[10px] h-6 px-2"
            >
              {endpoint.enabled ? "Disable" : "Enable"}
            </Button>
            <Button
              onClick={() => void handleTest()}
              loading={testPending}
              icon={<FlaskConical className="h-3 w-3" />}
              className="text-[10px] h-6 px-2"
            >
              Test
            </Button>
            <Button
              onClick={() => void handleDelete()}
              loading={deletePending}
              icon={<Trash2 className="h-3 w-3" />}
              className="text-[10px] h-6 px-2 text-danger hover:bg-danger/10"
            >
              Delete
            </Button>
          </div>
        </td>
      </tr>
      {expanded && (
        <tr className="bg-surface-raised/20 border-b border-border">
          <td colSpan={5} className="px-0 py-0">
            <div className="border-t border-border/40">
              <div className="px-4 py-2 text-[10px] uppercase tracking-wide text-text-muted font-medium">
                Delivery Logs (last 50)
              </div>
              <DeliveryLogsPanel endpointId={endpoint.id} />
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

function AddWebhookForm({ onClose }: { onClose: () => void }) {
  const { toast } = useToast();
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [selectedEvents, setSelectedEvents] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const { mutateAsync: createWebhook, isPending } = useCreateWebhook();

  function toggleEvent(value: string) {
    setSelectedEvents((prev) =>
      prev.includes(value) ? prev.filter((e) => e !== value) : [...prev, value],
    );
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!url.trim()) {
      setError("URL is required.");
      return;
    }
    if (selectedEvents.length === 0) {
      setError("Select at least one event type.");
      return;
    }
    try {
      await createWebhook({ url: url.trim(), secret: secret.trim() || undefined, events: selectedEvents });
      toast.success("Webhook created.");
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create webhook.");
    }
  }

  return (
    <div className="border-t border-border bg-surface-raised/20 px-4 py-4">
      <form onSubmit={(e) => void handleSubmit(e)} className="space-y-3">
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-[10px] uppercase tracking-wide text-text-muted mb-1">
              URL <span className="text-danger">*</span>
            </label>
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com/webhook"
              className={cn(
                "w-full px-2 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>
          <div>
            <label className="block text-[10px] uppercase tracking-wide text-text-muted mb-1">
              Secret (optional, auto-generated if empty)
            </label>
            <input
              type="text"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder="Leave blank to auto-generate"
              className={cn(
                "w-full px-2 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>
        </div>

        <div>
          <label className="block text-[10px] uppercase tracking-wide text-text-muted mb-1.5">
            Events <span className="text-danger">*</span>
          </label>
          <div className="flex flex-wrap gap-2">
            {WEBHOOK_EVENTS.map(({ value, label }) => (
              <label key={value} className="flex items-center gap-1.5 cursor-pointer">
                <input
                  type="checkbox"
                  checked={selectedEvents.includes(value)}
                  onChange={() => toggleEvent(value)}
                  className="h-3 w-3 rounded border-border"
                />
                <span className="text-xs text-text-secondary">{label}</span>
              </label>
            ))}
          </div>
        </div>

        {error && (
          <p className="text-xs text-danger flex items-center gap-1">
            <AlertCircle className="h-3 w-3 shrink-0" />
            {error}
          </p>
        )}

        <div className="flex items-center gap-2">
          <Button type="submit" loading={isPending} icon={<Plus className="h-3 w-3" />}>
            Create Webhook
          </Button>
          <Button type="button" onClick={onClose} className="text-text-muted">
            Cancel
          </Button>
        </div>
      </form>
    </div>
  );
}

function WebhooksCard() {
  const { data: webhooks = [], isLoading, error } = useWebhooks();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);

  function toggleExpand(id: string) {
    setExpandedId((prev) => (prev === id ? null : id));
  }

  return (
    <div className="bg-surface rounded-lg border border-border overflow-hidden">
      <div className="px-4 py-3 border-b border-border flex items-center gap-2">
        <Link2 className="h-4 w-4 text-text-muted" />
        <span className="text-xs font-medium text-text-primary">Webhooks</span>
        {!isLoading && (webhooks as WebhookEndpoint[]).length > 0 && (
          <span className="ml-auto text-[10px] text-text-muted">
            {(webhooks as WebhookEndpoint[]).length} endpoint
            {(webhooks as WebhookEndpoint[]).length !== 1 ? "s" : ""}
          </span>
        )}
        {!showAddForm && (
          <Button
            onClick={() => setShowAddForm(true)}
            icon={<Plus className="h-3 w-3" />}
            className={cn("text-[10px] h-6 px-2", !isLoading && (webhooks as WebhookEndpoint[]).length > 0 ? "" : "ml-auto")}
          >
            Add Webhook
          </Button>
        )}
      </div>

      {showAddForm && <AddWebhookForm onClose={() => setShowAddForm(false)} />}

      {error ? (
        <div className="flex items-center gap-2 text-danger text-xs px-4 py-3">
          <AlertCircle className="h-3.5 w-3.5 shrink-0" />
          <span>Failed to load webhooks.</span>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-border">
                {["URL", "Events", "Status", "Created", "Actions"].map((col) => (
                  <th
                    key={col}
                    className="text-left px-4 py-2 text-[10px] uppercase tracking-wide text-text-muted font-medium whitespace-nowrap"
                  >
                    {col}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {isLoading &&
                [1, 2].map((i) => (
                  <tr key={i} className="border-b border-border last:border-0">
                    {[1, 2, 3, 4, 5].map((j) => (
                      <td key={j} className="px-4 py-3">
                        <Skeleton className="h-3 w-24" />
                      </td>
                    ))}
                  </tr>
                ))}

              {!isLoading &&
                (webhooks as WebhookEndpoint[]).map((ep) => (
                  <WebhookRow
                    key={ep.id}
                    endpoint={ep}
                    expanded={expandedId === ep.id}
                    onToggleExpand={() => toggleExpand(ep.id)}
                  />
                ))}

              {!isLoading && (webhooks as WebhookEndpoint[]).length === 0 && !showAddForm && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-text-muted">
                    No webhook endpoints configured. Click{" "}
                    <button
                      type="button"
                      className="underline text-accent"
                      onClick={() => setShowAddForm(true)}
                    >
                      Add Webhook
                    </button>{" "}
                    to get started.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────────

export function AdminPage() {
  return (
    <div className="space-y-4 max-w-5xl">
      {/* Page header */}
      <div className="mb-2">
        <h1 className="text-sm font-semibold text-text-primary">Admin</h1>
        <p className="text-xs text-text-muted mt-0.5">
          System health, worker pool status, and server configuration.
        </p>
      </div>

      {/* Section 1 — System Status */}
      <SystemStatusCard />

      {/* Section 2 — Workers */}
      <WorkersCard />

      {/* Section 3 — Configuration */}
      <SettingsCard />

      {/* Section 4 — Log Viewer */}
      <div className="bg-surface rounded-lg border border-border overflow-hidden">
        <div className="px-4 py-3 border-b border-border flex items-center gap-2">
          <FileText className="h-4 w-4 text-text-muted" />
          <span className="text-xs font-medium text-text-primary">
            Log Viewer
          </span>
        </div>
        <div className="p-4">
          <LogViewer />
        </div>
      </div>

      {/* Section 5 — Webhooks */}
      <WebhooksCard />
    </div>
  );
}
