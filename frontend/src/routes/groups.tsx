import { useState, useCallback } from "react";
import { X, Plus, Pencil, Trash2, Users, Search } from "lucide-react";
import { Button } from "../components/button";
import { StatusBadge, Skeleton, PaginationBar } from "../components";
import { formatRelativeTime, cn } from "../lib/utils";
import { useToast } from "../components/toast-provider";
import {
  useGroups,
  useGroupMembers,
  useCreateGroup,
  useUpdateGroup,
  useDeleteGroup,
  useRemoveHostsFromGroup,
} from "../api/hooks/use-groups";
import type { HostGroup } from "../api/hooks/use-groups";

// ── Color palette ─────────────────────────────────────────────────────────────

const PRESET_COLORS = [
  "#6366f1", // indigo
  "#8b5cf6", // violet
  "#ec4899", // pink
  "#ef4444", // red
  "#f97316", // orange
  "#eab308", // yellow
  "#22c55e", // green
  "#14b8a6", // teal
  "#3b82f6", // blue
  "#06b6d4", // cyan
  "#84cc16", // lime
  "#64748b", // slate
];

// ── Group form modal ──────────────────────────────────────────────────────────

interface GroupFormModalProps {
  group?: HostGroup;
  onClose: () => void;
}

function GroupFormModal({ group, onClose }: GroupFormModalProps) {
  const { toast } = useToast();
  const [name, setName] = useState(group?.name ?? "");
  const [description, setDescription] = useState(group?.description ?? "");
  const [color, setColor] = useState(group?.color ?? PRESET_COLORS[0]!);
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: createGroup, isPending: isCreating } = useCreateGroup();
  const { mutateAsync: updateGroup, isPending: isUpdating } = useUpdateGroup();
  const isPending = isCreating || isUpdating;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Name is required.");
      return;
    }
    try {
      if (group) {
        await updateGroup({ id: group.id, body: { name: trimmedName, description: description.trim() || undefined, color } });
        toast.success("Group updated.");
      } else {
        await createGroup({ name: trimmedName, description: description.trim() || undefined, color });
        toast.success("Group created.");
      }
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save group.");
    }
  }

  return (
    <>
      <div
        className="fixed inset-0 bg-black/50 z-50"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-label={group ? "Edit group" : "Create group"}
        className={cn(
          "fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50",
          "w-full max-w-md bg-surface border border-border rounded-lg shadow-xl",
          "p-6 space-y-4",
        )}
      >
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-text-primary">
            {group ? "Edit Group" : "Create Group"}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close modal"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <form onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
          {/* Name */}
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1">
              Name <span className="text-danger">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Production servers"
              autoFocus
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Description */}
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description…"
              rows={2}
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border resize-none",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Color */}
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-2">
              Color
            </label>
            <div className="flex flex-wrap gap-2">
              {PRESET_COLORS.map((c) => (
                <button
                  key={c}
                  type="button"
                  aria-label={`Color ${c}`}
                  onClick={() => setColor(c)}
                  className={cn(
                    "h-6 w-6 rounded-full transition-transform hover:scale-110",
                    color === c && "ring-2 ring-offset-2 ring-offset-surface ring-text-primary scale-110",
                  )}
                  style={{ backgroundColor: c }}
                />
              ))}
            </div>
          </div>

          {error && <p className="text-xs text-danger">{error}</p>}

          <div className="flex justify-end gap-2 pt-1">
            <Button variant="secondary" onClick={onClose} type="button">
              Cancel
            </Button>
            <Button type="submit" loading={isPending} disabled={!name.trim()}>
              {group ? "Save changes" : "Create group"}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}

// ── Group detail panel ────────────────────────────────────────────────────────

interface GroupDetailPanelProps {
  group: HostGroup;
  onClose: () => void;
  onEdit: () => void;
}

const MEMBERS_PER_PAGE = 10;

function GroupDetailPanel({ group, onClose, onEdit }: GroupDetailPanelProps) {
  const { toast } = useToast();
  const [memberPage, setMemberPage] = useState(1);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [memberSearch, setMemberSearch] = useState("");

  const { data: membersData, isLoading: membersLoading } = useGroupMembers(group.id, {
    page: memberPage,
    page_size: MEMBERS_PER_PAGE,
  });
  const { mutateAsync: deleteGroup, isPending: isDeleting } = useDeleteGroup();
  const { mutateAsync: removeHosts, isPending: isRemoving } = useRemoveHostsFromGroup();

  const members = membersData?.data ?? [];
  const totalMemberPages = membersData?.pagination?.total_pages ?? 0;

  const filteredMembers = memberSearch.trim()
    ? members.filter(
        (m) =>
          m.ip_address.includes(memberSearch) ||
          m.hostname?.toLowerCase().includes(memberSearch.toLowerCase()),
      )
    : members;

  async function handleDelete() {
    try {
      await deleteGroup(group.id);
      toast.success("Group deleted.");
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete group.");
      setShowDeleteConfirm(false);
    }
  }

  async function handleRemoveMember(hostId: string) {
    try {
      await removeHosts({ groupId: group.id, hostIds: [hostId] });
      toast.success("Host removed from group.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to remove host.");
    }
  }

  return (
    <>
      <div
        className="fixed inset-0 bg-black/40 z-40"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-label="Group details"
        className={cn(
          "fixed top-0 right-0 bottom-0 z-50",
          "w-full max-w-110",
          "bg-surface border-l border-border",
          "flex flex-col overflow-hidden shadow-xl",
        )}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-border shrink-0">
          <div className="flex flex-col gap-1.5 min-w-0">
            <p className="text-xs text-text-muted">Group</p>
            <div className="flex items-center gap-2 min-w-0">
              {group.color && (
                <span
                  className="h-3 w-3 rounded-full shrink-0"
                  style={{ backgroundColor: group.color }}
                />
              )}
              <p className="text-sm font-medium text-text-primary truncate">
                {group.name}
              </p>
            </div>
            <p className="text-xs text-text-muted">
              {group.member_count} member{group.member_count !== 1 ? "s" : ""}
            </p>
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

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
          {/* Description */}
          {group.description && (
            <p className="text-xs text-text-secondary">{group.description}</p>
          )}

          {/* Meta */}
          <div className="space-y-1.5">
            <div className="flex gap-2 text-xs">
              <span className="text-text-muted w-24 shrink-0">Created</span>
              <span className="text-text-secondary">
                {formatRelativeTime(group.created_at)}
              </span>
            </div>
            <div className="flex gap-2 text-xs">
              <span className="text-text-muted w-24 shrink-0">Updated</span>
              <span className="text-text-secondary">
                {formatRelativeTime(group.updated_at)}
              </span>
            </div>
          </div>

          {/* Members */}
          <section>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-xs font-medium text-text-primary flex items-center gap-1.5">
                <Users className="h-3.5 w-3.5 text-text-muted" />
                Members
              </h3>
            </div>

            {/* Member search */}
            <div className="relative mb-2">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3 w-3 text-text-muted pointer-events-none" />
              <input
                type="text"
                placeholder="Search members…"
                value={memberSearch}
                onChange={(e) => setMemberSearch(e.target.value)}
                className={cn(
                  "w-full pl-7 pr-3 py-1 text-xs rounded border border-border",
                  "bg-surface text-text-primary placeholder:text-text-muted",
                  "focus:outline-none focus:ring-1 focus:ring-border",
                )}
                aria-label="Search members"
              />
            </div>

            {membersLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <div key={i} className="flex gap-2">
                    <Skeleton className="h-3 w-28" />
                    <Skeleton className="h-3 w-20" />
                  </div>
                ))}
              </div>
            ) : filteredMembers.length === 0 ? (
              <p className="text-xs text-text-muted">
                {members.length === 0 ? "No members in this group." : "No results."}
              </p>
            ) : (
              <div className="space-y-0.5">
                {filteredMembers.map((m) => (
                  <div
                    key={m.id}
                    className="flex items-center justify-between gap-2 py-1.5 border-b border-border/40 last:border-0"
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="font-mono text-xs text-text-primary shrink-0">
                        {m.ip_address}
                      </span>
                      {m.hostname && (
                        <span className="text-xs text-text-muted truncate">
                          {m.hostname}
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <StatusBadge status={m.status ?? "unknown"} />
                      <button
                        type="button"
                        aria-label={`Remove ${m.ip_address} from group`}
                        onClick={() => void handleRemoveMember(m.id)}
                        disabled={isRemoving}
                        className="p-0.5 rounded text-text-muted hover:text-danger hover:bg-danger/10 transition-colors"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {!membersLoading && totalMemberPages > 1 && (
              <PaginationBar
                page={memberPage}
                totalPages={totalMemberPages}
                onPageChange={setMemberPage}
                className="mt-3"
              />
            )}
          </section>
        </div>

        {/* Footer */}
        <div className="px-5 py-3 border-t border-border shrink-0 space-y-2">
          <Button
            icon={<Pencil className="h-3.5 w-3.5" />}
            onClick={onEdit}
            className="w-full justify-center"
          >
            Edit group
          </Button>

          {!showDeleteConfirm ? (
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              className="w-full flex items-center justify-center gap-1.5 text-xs text-text-muted hover:text-danger transition-colors py-1"
            >
              <Trash2 className="h-3 w-3" />
              Delete group
            </button>
          ) : (
            <div className="flex items-center justify-center gap-2">
              <span className="text-xs text-text-muted">Delete this group?</span>
              <Button
                variant="danger"
                onClick={() => void handleDelete()}
                loading={isDeleting}
                className="text-xs h-6 px-2"
              >
                Confirm
              </Button>
              <Button
                variant="secondary"
                onClick={() => setShowDeleteConfirm(false)}
                className="text-xs h-6 px-2"
              >
                Cancel
              </Button>
            </div>
          )}
        </div>
      </div>
    </>
  );
}

// ── Groups page ───────────────────────────────────────────────────────────────

export function GroupsPage() {
  const { toast } = useToast();
  const [selectedGroup, setSelectedGroup] = useState<HostGroup | null>(null);
  const [editingGroup, setEditingGroup] = useState<HostGroup | undefined>(undefined);
  const [showFormModal, setShowFormModal] = useState(false);
  const [search, setSearch] = useState("");

  const { data: groups = [], isLoading } = useGroups();

  const filteredGroups = search.trim()
    ? groups.filter(
        (g) =>
          g.name.toLowerCase().includes(search.toLowerCase()) ||
          g.description?.toLowerCase().includes(search.toLowerCase()),
      )
    : groups;

  const handleEdit = useCallback((group: HostGroup) => {
    setEditingGroup(group);
    setShowFormModal(true);
  }, []);

  const handleCloseForm = useCallback(() => {
    setShowFormModal(false);
    setEditingGroup(undefined);
  }, []);

  void toast; // used in sub-components

  return (
    <>
      <div className="space-y-4">
        {/* Toolbar */}
        <div className="flex items-center gap-3 flex-wrap">
          <div className="relative flex-1 min-w-40 max-w-sm">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-text-muted pointer-events-none" />
            <input
              type="text"
              placeholder="Search groups…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className={cn(
                "w-full pl-8 pr-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
              aria-label="Search groups"
            />
          </div>
          <Button
            icon={<Plus className="h-3.5 w-3.5" />}
            onClick={() => {
              setEditingGroup(undefined);
              setShowFormModal(true);
            }}
            className="sm:ml-auto"
          >
            Create group
          </Button>
        </div>

        {/* Table */}
        <div className="bg-surface rounded-lg border border-border overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border bg-surface">
                  <th className="text-left font-medium text-text-muted py-3 px-4">
                    Name
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Description
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Members
                  </th>
                  <th className="text-left font-medium text-text-muted py-3 pr-4">
                    Updated
                  </th>
                  <th className="py-3" />
                </tr>
              </thead>
              <tbody>
                {isLoading
                  ? Array.from({ length: 6 }).map((_, i) => (
                      <tr key={i} className="border-b border-border/50">
                        <td className="py-3 px-4">
                          <div className="flex items-center gap-2">
                            <Skeleton className="h-3 w-3 rounded-full" />
                            <Skeleton className="h-3.5 w-28" />
                          </div>
                        </td>
                        <td className="py-3 pr-4">
                          <Skeleton className="h-3.5 w-40" />
                        </td>
                        <td className="py-3 pr-4">
                          <Skeleton className="h-3.5 w-8" />
                        </td>
                        <td className="py-3 pr-4">
                          <Skeleton className="h-3.5 w-16" />
                        </td>
                        <td className="py-3" />
                      </tr>
                    ))
                  : filteredGroups.length === 0
                    ? (
                      <tr>
                        <td
                          colSpan={5}
                          className="py-12 text-center text-text-muted"
                        >
                          {search ? "No groups match your search." : "No groups found."}
                        </td>
                      </tr>
                    )
                    : filteredGroups.map((group) => (
                      <tr
                        key={group.id}
                        onClick={() => setSelectedGroup(group)}
                        className={cn(
                          "border-b border-border/50 cursor-pointer transition-colors",
                          "hover:bg-surface-raised",
                          selectedGroup?.id === group.id && "bg-surface-raised",
                        )}
                      >
                        <td className="py-3 px-4">
                          <div className="flex items-center gap-2">
                            <span
                              className="h-2.5 w-2.5 rounded-full shrink-0"
                              style={{ backgroundColor: group.color ?? "#64748b" }}
                            />
                            <span className="font-medium text-text-primary">
                              {group.name}
                            </span>
                          </div>
                        </td>
                        <td className="py-3 pr-4 text-text-secondary max-w-xs truncate">
                          {group.description ?? "—"}
                        </td>
                        <td className="py-3 pr-4 text-text-secondary tabular-nums">
                          {group.member_count}
                        </td>
                        <td className="py-3 pr-4 text-text-muted whitespace-nowrap">
                          {formatRelativeTime(group.updated_at)}
                        </td>
                        <td className="py-3 pr-4">
                          <button
                            type="button"
                            aria-label={`Edit ${group.name}`}
                            onClick={(e) => {
                              e.stopPropagation();
                              handleEdit(group);
                            }}
                            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface transition-colors opacity-0 group-hover:opacity-100"
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </button>
                        </td>
                      </tr>
                    ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* Detail panel */}
      {selectedGroup && (
        <GroupDetailPanel
          group={selectedGroup}
          onClose={() => setSelectedGroup(null)}
          onEdit={() => handleEdit(selectedGroup)}
        />
      )}

      {/* Create / edit modal */}
      {showFormModal && (
        <GroupFormModal group={editingGroup} onClose={handleCloseForm} />
      )}
    </>
  );
}
