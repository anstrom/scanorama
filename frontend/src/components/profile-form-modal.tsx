import { useState, useId } from "react";
import { X } from "lucide-react";
import { Button } from "./button";
import { useCreateProfile, useUpdateProfile } from "../api/hooks/use-profiles";
import { useToast } from "./toast-provider";
import { cn } from "../lib/utils";
import type { components } from "../api/types";

type CreateProfileRequest = components["schemas"]["docs.CreateProfileRequest"];

const SCAN_TYPES = [
  { value: "connect", label: "Connect (-sT)" },
  { value: "syn", label: "SYN stealth (-sS)" },
  { value: "ack", label: "ACK (-sA)" },
  { value: "aggressive", label: "Aggressive (-sS -sV -A)" },
  { value: "comprehensive", label: "Comprehensive (-sS -sV --script=default)" },
] as const;

export interface ProfileFormModalProps {
  mode: "create" | "edit";
  initial?: {
    id?: string;
    name?: string;
    description?: string;
    scan_type?: string;
    ports?: string;
  };
  onClose: () => void;
  onSaved?: () => void;
}

export function ProfileFormModal({
  mode,
  initial,
  onClose,
  onSaved,
}: ProfileFormModalProps) {
  const id = useId();

  const [name, setName] = useState(initial?.name ?? "");
  const [scanType, setScanType] = useState(initial?.scan_type ?? "connect");
  const [ports, setPorts] = useState(initial?.ports ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [error, setError] = useState<string | null>(null);

  const { toast } = useToast();
  const { mutateAsync: createProfile, isPending: isCreating } =
    useCreateProfile();
  const { mutateAsync: updateProfile, isPending: isUpdating } =
    useUpdateProfile();
  const isPending = isCreating || isUpdating;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Name is required.");
      return;
    }

    const body: CreateProfileRequest = {
      name: trimmedName,
      scan_type: scanType as CreateProfileRequest["scan_type"],
      ports: ports.trim() || undefined,
      description: description.trim() || undefined,
    };

    try {
      if (mode === "create") {
        await createProfile(body);
        toast.success("Profile created");
      } else {
        await updateProfile({ id: initial?.id ?? "", body });
        toast.success("Profile updated");
      }
      onSaved?.();
      onClose();
    } catch (err) {
      const apiErr = err as { message?: string; error?: string };
      const msg =
        apiErr.message ??
        apiErr.error ??
        (mode === "create"
          ? "Failed to create profile."
          : "Failed to update profile.");
      setError(msg);
      toast.error(msg);
    }
  }

  const title = mode === "create" ? "Create Profile" : "Edit Profile";

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50 z-40"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Dialog */}
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        className={cn(
          "fixed z-50 inset-0 m-auto",
          "w-full max-w-md h-fit",
          "bg-surface border border-border rounded-lg shadow-xl",
          "flex flex-col",
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-border">
          <h2
            id={`${id}-title`}
            className="text-sm font-semibold text-text-primary"
          >
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close dialog"
            className="p-1 rounded text-text-muted hover:text-text-primary hover:bg-surface-raised transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="px-5 py-4 space-y-5">
          {/* Name */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-name`}
              className="block text-xs font-medium text-text-primary"
            >
              Name
            </label>
            <input
              id={`${id}-name`}
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Quick scan"
              autoFocus
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Scan type */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-scan-type`}
              className="block text-xs font-medium text-text-primary"
            >
              Scan type
            </label>
            <select
              id={`${id}-scan-type`}
              value={scanType}
              onChange={(e) => setScanType(e.target.value)}
              aria-label="Select scan type"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            >
              {SCAN_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>

          {/* Ports */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-ports`}
              className="block text-xs font-medium text-text-primary"
            >
              Ports{" "}
              <span className="text-text-muted font-normal">(optional)</span>
            </label>
            <input
              id={`${id}-ports`}
              type="text"
              value={ports}
              onChange={(e) => setPorts(e.target.value)}
              placeholder="22,80,443 or 1-65535"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border font-mono",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <label
              htmlFor={`${id}-description`}
              className="block text-xs font-medium text-text-primary"
            >
              Description{" "}
              <span className="text-text-muted font-normal">(optional)</span>
            </label>
            <input
              id={`${id}-description`}
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Fast TCP connect scan"
              className={cn(
                "w-full px-3 py-1.5 text-xs rounded border border-border",
                "bg-surface text-text-primary placeholder:text-text-muted",
                "focus:outline-none focus:ring-1 focus:ring-border",
              )}
            />
          </div>

          {/* Inline error */}
          {error && (
            <p role="alert" className="text-xs text-danger">
              {error}
            </p>
          )}

          {/* Footer */}
          <div className="flex justify-end gap-2 pt-1">
            <Button variant="secondary" type="button" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={isPending}>
              {mode === "create" ? "Create profile" : "Save changes"}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
