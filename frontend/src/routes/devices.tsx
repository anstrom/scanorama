import { X, Cpu, Wifi, Tag, Pencil, Trash2, Link2Off } from "lucide-react";
import { useState } from "react";
import { Button } from "../components/button";
import { StatusBadge, Skeleton } from "../components";
import { formatRelativeTime, cn } from "../lib/utils";
import { useToast } from "../components/toast-provider";
import { getErrorMessage } from "../api/errors";
import {
  useDevice,
  useUpdateDevice,
  useDeleteDevice,
  useDetachHost,
} from "../api/hooks/use-devices";

// ── Source badge ──────────────────────────────────────────────────────────────

const SOURCE_LABEL: Record<string, string> = {
  mdns: "mDNS",
  dns: "DNS",
  snmp: "SNMP",
  netbios: "NetBIOS",
  user: "User",
};

function SourceBadge({ source }: { source: string }) {
  return (
    <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-surface-raised text-text-muted border border-border/60">
      {SOURCE_LABEL[source] ?? source}
    </span>
  );
}

// ── Edit name modal ───────────────────────────────────────────────────────────

interface EditNameModalProps {
  id: string;
  currentName: string;
  onClose: () => void;
}

function EditNameModal({ id, currentName, onClose }: EditNameModalProps) {
  const { toast } = useToast();
  const [name, setName] = useState(currentName);
  const [error, setError] = useState<string | null>(null);
  const { mutateAsync: updateDevice, isPending } = useUpdateDevice();

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) { setError("Name is required."); return; }
    try {
      await updateDevice({ id, body: { name: trimmed } });
    } catch (err) {
      setError(getErrorMessage(err, "Failed to rename device."));
      return;
    }
    toast.success("Device renamed.");
    onClose();
  }

  return (
    <>
      <div className="fixed inset-0 bg-black/50 z-50" onClick={onClose} aria-hidden="true" />
      <div
        role="dialog"
        aria-label="Rename device"
        className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-sm bg-surface border border-border rounded-lg shadow-xl p-6 space-y-4"
      >
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-text-primary">Rename Device</h2>
          <button type="button" onClick={onClose} aria-label="Close" className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors">
            <X className="h-4 w-4" />
          </button>
        </div>
        <form onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            className={cn(
              "w-full px-3 py-1.5 text-xs rounded border border-border",
              "bg-surface text-text-primary placeholder:text-text-muted",
              "focus:outline-none focus:ring-1 focus:ring-border",
            )}
          />
          {error && <p className="text-xs text-danger">{error}</p>}
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={onClose} type="button">Cancel</Button>
            <Button type="submit" loading={isPending} disabled={!name.trim()}>Save</Button>
          </div>
        </form>
      </div>
    </>
  );
}

// ── Device detail page ────────────────────────────────────────────────────────

interface DeviceDetailPageProps {
  id: string;
  onClose?: () => void;
}

export function DeviceDetailPage({ id, onClose }: DeviceDetailPageProps) {
  const { toast } = useToast();
  const [showEditModal, setShowEditModal] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  const { data: device, isLoading, isError } = useDevice(id);
  const { mutateAsync: deleteDevice, isPending: isDeleting } = useDeleteDevice();
  const { mutateAsync: detachHost, isPending: isDetaching } = useDetachHost();

  async function handleDelete() {
    try {
      await deleteDevice(id);
    } catch (err) {
      toast.error(getErrorMessage(err, "Failed to delete device."));
      setShowDeleteConfirm(false);
      return;
    }
    toast.success("Device deleted.");
    onClose?.();
  }

  async function handleDetach(hostId: string) {
    try {
      await detachHost({ deviceId: id, hostId });
      toast.success("Host detached.");
    } catch (err) {
      toast.error(getErrorMessage(err, "Failed to detach host."));
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-6 p-6">
        <Skeleton className="h-5 w-48" />
        <div className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-4 w-full" />
          ))}
        </div>
      </div>
    );
  }

  if (isError || !device) {
    return (
      <div className="p-6 text-sm text-text-muted">
        Failed to load device.
      </div>
    );
  }

  const knownMacs = device.known_macs ?? [];
  const knownNames = device.known_names ?? [];
  const hosts = device.hosts ?? [];

  return (
    <>
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <Cpu className="h-4 w-4 text-text-muted shrink-0" />
            <h1 className="text-sm font-semibold text-text-primary truncate">{device.name}</h1>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            <button
              type="button"
              aria-label="Rename device"
              onClick={() => setShowEditModal(true)}
              className="p-1.5 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
            >
              <Pencil className="h-3.5 w-3.5" />
            </button>
            {onClose && (
              <button
                type="button"
                aria-label="Close"
                onClick={onClose}
                className="p-1.5 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </div>

        {/* Meta */}
        <div className="space-y-1.5 text-xs">
          <div className="flex gap-2">
            <span className="text-text-muted w-24 shrink-0">Created</span>
            <span className="text-text-secondary">{formatRelativeTime(device.created_at ?? "")}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-text-muted w-24 shrink-0">Updated</span>
            <span className="text-text-secondary">{formatRelativeTime(device.updated_at ?? "")}</span>
          </div>
          {device.notes && (
            <p className="text-text-secondary pt-1">{device.notes}</p>
          )}
        </div>

        {/* Known MACs */}
        <section>
          <h2 className="text-xs font-medium text-text-primary flex items-center gap-1.5 mb-2">
            <Wifi className="h-3.5 w-3.5 text-text-muted" />
            Known MACs
          </h2>
          {knownMacs.length === 0 ? (
            <p className="text-xs text-text-muted">No MACs recorded.</p>
          ) : (
            <div className="space-y-1">
              {knownMacs.map((m) => (
                <div key={m.id} className="flex items-center justify-between gap-2 py-1 border-b border-border/40 last:border-0">
                  <span className="font-mono text-xs text-text-primary">{m.mac_address}</span>
                  <span className="text-[10px] text-text-muted whitespace-nowrap">
                    last seen {formatRelativeTime(m.last_seen ?? "")}
                  </span>
                </div>
              ))}
            </div>
          )}
        </section>

        {/* Known Names */}
        <section>
          <h2 className="text-xs font-medium text-text-primary flex items-center gap-1.5 mb-2">
            <Tag className="h-3.5 w-3.5 text-text-muted" />
            Known Names
          </h2>
          {knownNames.length === 0 ? (
            <p className="text-xs text-text-muted">No names recorded.</p>
          ) : (
            <div className="space-y-1">
              {knownNames.map((n) => (
                <div key={n.id} className="flex items-center justify-between gap-2 py-1 border-b border-border/40 last:border-0">
                  <span className="text-xs text-text-primary truncate">{n.name}</span>
                  <SourceBadge source={n.source ?? ""} />
                </div>
              ))}
            </div>
          )}
        </section>

        {/* Attached hosts */}
        <section>
          <h2 className="text-xs font-medium text-text-primary mb-2">Attached Hosts</h2>
          {hosts.length === 0 ? (
            <p className="text-xs text-text-muted">No hosts attached.</p>
          ) : (
            <div className="space-y-0.5">
              {hosts.map((h) => (
                <div key={h.id} className="flex items-center justify-between gap-2 py-1.5 border-b border-border/40 last:border-0">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="font-mono text-xs text-text-primary shrink-0">{h.ip_address}</span>
                    {h.hostname && (
                      <span className="text-xs text-text-muted truncate">{h.hostname}</span>
                    )}
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <StatusBadge status={h.status ?? "unknown"} />
                    <button
                      type="button"
                      aria-label={`Detach ${h.ip_address}`}
                      onClick={() => { if (h.id) void handleDetach(h.id); }}
                      disabled={isDetaching}
                      className="p-0.5 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
                    >
                      <Link2Off className="h-3 w-3" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        {/* Delete */}
        <div className="pt-2 border-t border-border">
          {!showDeleteConfirm ? (
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              className="flex items-center gap-1.5 text-xs text-text-muted hover:text-danger transition-colors"
            >
              <Trash2 className="h-3 w-3" />
              Delete device
            </button>
          ) : (
            <div className="flex items-center gap-2">
              <span className="text-xs text-text-muted">Delete this device?</span>
              <Button variant="danger" onClick={() => void handleDelete()} loading={isDeleting} className="text-xs h-6 px-2">
                Confirm
              </Button>
              <Button variant="secondary" onClick={() => setShowDeleteConfirm(false)} className="text-xs h-6 px-2">
                Cancel
              </Button>
            </div>
          )}
        </div>
      </div>

      {showEditModal && (
        <EditNameModal
          id={id}
          currentName={device.name ?? ""}
          onClose={() => setShowEditModal(false)}
        />
      )}
    </>
  );
}
