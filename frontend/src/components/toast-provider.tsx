/* eslint-disable react-refresh/only-export-components -- intentional: hook and provider are co-located by design */
import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { CheckCircle2, X, XCircle } from "lucide-react";
import { cn } from "../lib/utils";

// ── Types ──────────────────────────────────────────────────────────────────────

type ToastVariant = "success" | "error";

interface ToastItem {
  id: string;
  message: string;
  variant: ToastVariant;
  duration: number;
}

interface ToastContextValue {
  toast: {
    success(message: string): void;
    error(message: string): void;
  };
}

// ── Constants ──────────────────────────────────────────────────────────────────

const MAX_TOASTS = 4;
const SUCCESS_DURATION = 4_000;
const ERROR_DURATION = 6_000;

// ── Context ────────────────────────────────────────────────────────────────────

const noopToast = { success: () => {}, error: () => {} };
const ToastContext = createContext<ToastContextValue>({ toast: noopToast });

// ── Toast card ─────────────────────────────────────────────────────────────────

interface ToastCardProps {
  item: ToastItem;
  onDismiss: (id: string) => void;
}

function ToastCard({ item, onDismiss }: ToastCardProps) {
  const isSuccess = item.variant === "success";

  // Auto-dismiss after `duration` ms
  useEffect(() => {
    const timer = setTimeout(() => onDismiss(item.id), item.duration);
    return () => clearTimeout(timer);
  }, [item.id, item.duration, onDismiss]);

  return (
    <div
      role="alert"
      aria-live="assertive"
      className={cn(
        // Base card
        "relative overflow-hidden pointer-events-auto",
        "bg-surface border border-border rounded-lg shadow-lg",
        "px-4 py-3 flex items-start gap-3",
        "min-w-[240px] max-w-[360px] text-xs",
        // Left accent border
        "border-l-4",
        isSuccess ? "border-l-success" : "border-l-danger",
        // Entrance animation
        "transition-all duration-200",
      )}
    >
      {/* Icon */}
      {isSuccess ? (
        <CheckCircle2 className="h-3.5 w-3.5 text-success shrink-0 mt-0.5" />
      ) : (
        <XCircle className="h-3.5 w-3.5 text-danger shrink-0 mt-0.5" />
      )}

      {/* Message */}
      <span className="text-text-secondary flex-1 leading-relaxed">
        {item.message}
      </span>

      {/* Close button */}
      <button
        type="button"
        aria-label="Dismiss notification"
        onClick={() => onDismiss(item.id)}
        className="text-text-muted hover:text-text-primary transition-colors shrink-0 mt-0.5"
      >
        <X className="h-3 w-3" />
      </button>

      {/* Auto-dismiss progress bar — animates width 100% → 0% */}
      <div
        className="absolute bottom-0 left-0 h-[2px] opacity-50"
        style={{
          animationName: "toast-progress",
          animationDuration: `${item.duration}ms`,
          animationTimingFunction: "linear",
          animationFillMode: "forwards",
          backgroundColor: isSuccess
            ? "var(--color-success)"
            : "var(--color-danger)",
        }}
      />
    </div>
  );
}

// ── Provider ───────────────────────────────────────────────────────────────────

export function ToastProvider({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const addToast = useCallback((message: string, variant: ToastVariant) => {
    const duration = variant === "success" ? SUCCESS_DURATION : ERROR_DURATION;
    const newToast: ToastItem = {
      id: crypto.randomUUID(),
      message,
      variant,
      duration,
    };
    setToasts((prev) => {
      const next = [...prev, newToast];
      // Oldest entries are dropped from the front when the cap is exceeded
      return next.length > MAX_TOASTS
        ? next.slice(next.length - MAX_TOASTS)
        : next;
    });
  }, []);

  const toastSuccess = useCallback(
    (message: string) => addToast(message, "success"),
    [addToast],
  );

  const toastError = useCallback(
    (message: string) => addToast(message, "error"),
    [addToast],
  );

  const contextValue = useMemo<ToastContextValue>(
    () => ({ toast: { success: toastSuccess, error: toastError } }),
    [toastSuccess, toastError],
  );

  return (
    <ToastContext.Provider value={contextValue}>
      {/* Keyframe for the progress-bar shrink animation */}
      <style>{`
        @keyframes toast-progress {
          from { width: 100%; }
          to   { width: 0%;   }
        }
      `}</style>

      {children}

      {/* Fixed toast container — bottom-right, above everything */}
      <div
        className="fixed bottom-4 right-4 z-[60] flex flex-col gap-2 pointer-events-none"
        aria-label="Notifications"
      >
        {toasts.map((t) => (
          <ToastCard key={t.id} item={t} onDismiss={dismiss} />
        ))}
      </div>
    </ToastContext.Provider>
  );
}

// ── Hook ───────────────────────────────────────────────────────────────────────

export function useToast() {
  return useContext(ToastContext);
}
