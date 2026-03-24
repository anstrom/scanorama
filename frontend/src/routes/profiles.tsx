import { useState, useCallback } from "react";
import { Plus, Pencil, Trash2, X, SlidersHorizontal } from "lucide-react";
import { Button } from "../components/button";
import { useProfiles, useDeleteProfile } from "../api/hooks/use-profiles";
import { useToast } from "../components/toast-provider";
import { Skeleton, PaginationBar } from "../components";
import { ProfileFormModal } from "../components/profile-form-modal";
import { formatRelativeTime, formatAbsoluteTime, cn } from "../lib/utils";
import type { components } from "../api/types";

type ProfileResponse = components["schemas"]["docs.ProfileResponse"];

const PAGE_SIZE = 25;

// ── Helpers ───────────────────────────────────────────────────────────────────

const SCAN_TYPE_LABELS: Record<string, string> = {
  connect: "Connect (-sT)",
  syn: "SYN stealth (-sS)",
  ack: "ACK (-sA)",
  udp: "UDP (-sU)",
  aggressive: "Aggressive (-sS -sV -A)",
  comprehensive: "Comprehensive (-sS -sV --script=default)",
};

const SCAN_TYPE_SHORT_LABELS: Record<string, string> = {
  connect: "Connect",
  syn: "SYN",
  ack: "ACK",
  udp: "UDP",
  aggressive: "Aggressive",
  comprehensive: "Comprehensive",
};

// ── Skeleton rows ─────────────────────────────────────────────────────────────

function SkeletonRows() {
  return (
    <>
      {Array.from({ length: 6 }).map((_, i) => (
        <tr key={i} className="border-b border-border">
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-32" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-20" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-28 font-mono" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-40" />
          </td>
          <td className="px-4 py-2.5">
            <Skeleton className="h-3 w-16" />
          </td>
        </tr>
      ))}
    </>
  );
}

// ── Meta row helper ───────────────────────────────────────────────────────────

function MetaRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex gap-2 text-xs">
      <span className="text-text-muted w-32 shrink-0">{label}</span>
      <span className="text-text-secondary break-all">{value ?? "—"}</span>
    </div>
  );
}

// ── Profile detail panel ──────────────────────────────────────────────────────

interface ProfileDetailPanelProps {
  profile: ProfileResponse;
  onClose: () => void;
}

function ProfileDetailPanel({
  profile: initialProfile,
  onClose,
}: ProfileDetailPanelProps) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [showEditModal, setShowEditModal] = useState(false);

  const { mutateAsync: deleteProfile, isPending: isDeleting } =
    useDeleteProfile();

  const { toast } = useToast();
  const p = initialProfile;

  async function handleDelete() {
    setActionError(null);
    try {
      await deleteProfile(p.id ?? "");
      toast.success("Profile deleted");
      onClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Delete failed.";
      setActionError(msg);
      setShowDeleteConfirm(false);
      toast.error(msg);
    }
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Panel */}
      <div
        role="dialog"
        aria-label="Profile details"
        className={cn(
          "fixed top-0 right-0 bottom-0 z-50",
          "w-full max-w-110",
          "bg-surface border-l border-border",
          "flex flex-col overflow-hidden",
          "shadow-xl",
        )}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-border shrink-0">
          <div className="flex flex-col gap-1.5 min-w-0">
            <p className="text-xs text-text-muted">Profile</p>
            <p className="text-sm font-medium text-text-primary truncate">
              {p.name ?? "—"}
            </p>
            {p.scan_type && (
              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-medium bg-accent/15 text-accent w-fit">
                {SCAN_TYPE_SHORT_LABELS[p.scan_type] ?? p.scan_type}
              </span>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close panel"
            className="shrink-0 p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Action bar */}
        <div className="flex items-center gap-2 px-5 py-3 border-b border-border shrink-0 flex-wrap">
          <Button
            variant="secondary"
            onClick={() => setShowEditModal(true)}
            className="text-xs h-7 px-3"
          >
            <Pencil className="h-3 w-3 mr-1" />
            Edit
          </Button>

          {showDeleteConfirm ? (
            <div className="flex items-center gap-2 ml-auto">
              <span className="text-xs text-text-muted">
                Delete this profile?
              </span>
              <button
                type="button"
                onClick={() => setShowDeleteConfirm(false)}
                className="text-xs text-text-muted hover:text-text-secondary"
              >
                Cancel
              </button>
              <Button
                variant="danger"
                onClick={() => void handleDelete()}
                loading={isDeleting}
                className="text-xs h-7 px-3"
              >
                Delete
              </Button>
            </div>
          ) : (
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              className="ml-auto flex items-center gap-1 text-xs text-text-muted hover:text-danger transition-colors"
            >
              <Trash2 className="h-3 w-3" />
              Delete
            </button>
          )}

          {actionError && (
            <p className="w-full text-[11px] text-danger mt-0.5">
              {actionError}
            </p>
          )}
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-6">
          {/* Configuration */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3 flex items-center gap-1.5">
              <SlidersHorizontal className="h-3.5 w-3.5 text-text-muted" />
              Configuration
            </h3>
            <div className="space-y-2">
              <MetaRow
                label="Scan type"
                value={
                  p.scan_type
                    ? (SCAN_TYPE_LABELS[p.scan_type] ?? p.scan_type)
                    : undefined
                }
              />
              <MetaRow
                label="Ports"
                value={
                  p.ports ? (
                    <span className="font-mono">{p.ports}</span>
                  ) : undefined
                }
              />
              <MetaRow label="Description" value={p.description} />
              <MetaRow label="ID" value={p.id} />
            </div>
          </section>

          {/* Timestamps */}
          <section>
            <h3 className="text-xs font-medium text-text-primary mb-3">
              Timestamps
            </h3>
            <div className="space-y-2">
              <MetaRow
                label="Created at"
                value={
                  p.created_at ? formatAbsoluteTime(p.created_at) : undefined
                }
              />
              <MetaRow
                label="Updated at"
                value={
                  p.updated_at ? formatAbsoluteTime(p.updated_at) : undefined
                }
              />
            </div>
          </section>
        </div>
      </div>

      {/* Edit modal — rendered outside the panel div so z-index stacking works */}
      {showEditModal && (
        <ProfileFormModal
          mode="edit"
          initial={{
            id: p.id,
            name: p.name,
            description: p.description,
            scan_type: p.scan_type,
            ports: p.ports,
          }}
          onClose={() => setShowEditModal(false)}
          onSaved={() => setShowEditModal(false)}
        />
      )}
    </>
  );
}

// ── Profiles page ─────────────────────────────────────────────────────────────

export function ProfilesPage() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [selectedProfile, setSelectedProfile] =
    useState<ProfileResponse | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);

  // Debounce name search
  const debounceRef = {
    current: 0 as unknown as ReturnType<typeof setTimeout>,
  };
  const handleSearchInput = useCallback(
    (value: string) => {
      setSearch(value);
      clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => {
        setDebouncedSearch(value);
        setPage(1);
      }, 300);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const { data, isLoading } = useProfiles({ page, page_size: PAGE_SIZE });

  const allProfiles = data?.data ?? [];
  const pagination = data?.pagination;
  const totalPages = pagination?.total_pages ?? 1;

  // Client-side name filter — the API doesn't expose a search query param
  const profiles = debouncedSearch.trim()
    ? allProfiles.filter((p) =>
        (p.name ?? "")
          .toLowerCase()
          .includes(debouncedSearch.trim().toLowerCase()),
      )
    : allProfiles;

  return (
    <div className="flex flex-col gap-4 h-full">
      {/* Toolbar */}
      <div className="flex items-center gap-3 flex-wrap">
        {/* Search */}
        <div className="relative flex-1 min-w-48 max-w-64">
          <input
            type="text"
            value={search}
            onChange={(e) => handleSearchInput(e.target.value)}
            placeholder="Search by name…"
            aria-label="Search by name…"
            className={cn(
              "w-full pl-3 pr-3 py-1.5 text-xs rounded border border-border",
              "bg-surface text-text-primary placeholder:text-text-muted",
              "focus:outline-none focus:ring-1 focus:ring-border",
            )}
          />
        </div>

        {/* Spacer */}
        <div className="flex-1" />

        {/* Create profile */}
        <Button
          onClick={() => setShowCreateModal(true)}
          className="text-xs h-7 px-3"
        >
          <Plus className="h-3 w-3 mr-1" />
          Create Profile
        </Button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto rounded border border-border">
        <table className="w-full text-xs border-collapse min-w-[640px]">
          <thead>
            <tr className="bg-surface-raised border-b border-border text-left">
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Name
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Scan Type
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Ports
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Description
              </th>
              <th className="px-4 py-2.5 font-medium text-text-secondary whitespace-nowrap">
                Updated
              </th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <SkeletonRows />
            ) : profiles.length === 0 ? (
              <tr>
                <td
                  colSpan={5}
                  className="px-4 py-10 text-center text-text-muted"
                >
                  No profiles found.
                </td>
              </tr>
            ) : (
              profiles.map((profile) => (
                <tr
                  key={profile.id}
                  onClick={() => setSelectedProfile(profile)}
                  className={cn(
                    "border-b border-border cursor-pointer transition-colors",
                    "hover:bg-surface-raised",
                    selectedProfile?.id === profile.id && "bg-accent/8",
                  )}
                >
                  <td className="px-4 py-2.5 text-text-primary font-medium truncate max-w-[180px]">
                    {profile.name ?? "—"}
                  </td>
                  <td className="px-4 py-2.5 text-text-secondary">
                    {profile.scan_type
                      ? (SCAN_TYPE_SHORT_LABELS[profile.scan_type] ??
                        profile.scan_type)
                      : "—"}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-text-secondary whitespace-nowrap">
                    {profile.ports ?? "—"}
                  </td>
                  <td className="px-4 py-2.5 text-text-secondary truncate max-w-[200px]">
                    {profile.description ?? "—"}
                  </td>
                  <td className="px-4 py-2.5 text-text-muted whitespace-nowrap">
                    {profile.updated_at
                      ? formatRelativeTime(profile.updated_at)
                      : "—"}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {!isLoading && profiles.length > 0 && totalPages > 1 && (
        <PaginationBar
          page={page}
          totalPages={totalPages}
          onPrev={() => setPage((p) => Math.max(1, p - 1))}
          onNext={() => setPage((p) => Math.min(totalPages, p + 1))}
        />
      )}

      {/* Profile detail panel */}
      {selectedProfile && (
        <ProfileDetailPanel
          profile={selectedProfile}
          onClose={() => setSelectedProfile(null)}
        />
      )}

      {/* Create modal */}
      {showCreateModal && (
        <ProfileFormModal
          mode="create"
          onClose={() => setShowCreateModal(false)}
        />
      )}
    </div>
  );
}
