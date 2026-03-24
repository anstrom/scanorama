import { render, screen, act, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, afterEach } from "vitest";
import { ToastProvider, useToast } from "./toast-provider";

// ── Helper components ─────────────────────────────────────────────────────────

function Trigger({
  variant,
  message,
}: {
  variant: "success" | "error";
  message: string;
}) {
  const { toast } = useToast();
  return (
    <button
      type="button"
      onClick={() =>
        variant === "success" ? toast.success(message) : toast.error(message)
      }
    >
      Trigger
    </button>
  );
}

function renderToast(variant: "success" | "error", message: string) {
  return render(
    <ToastProvider>
      <Trigger variant={variant} message={message} />
    </ToastProvider>,
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("ToastProvider + useToast", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders success toast message in the DOM", async () => {
    const user = userEvent.setup();
    renderToast("success", "Operation successful");
    await user.click(screen.getByRole("button", { name: "Trigger" }));
    expect(screen.getByText("Operation successful")).toBeInTheDocument();
  });

  it("renders error toast message in the DOM", async () => {
    const user = userEvent.setup();
    renderToast("error", "Something went wrong");
    await user.click(screen.getByRole("button", { name: "Trigger" }));
    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("success toast has success border styling", async () => {
    const user = userEvent.setup();
    renderToast("success", "Success!");
    await user.click(screen.getByRole("button", { name: "Trigger" }));
    const alert = screen.getByRole("alert");
    expect(alert.className).toContain("border-l-success");
  });

  it("error toast close button dismisses the toast", async () => {
    const user = userEvent.setup();
    renderToast("error", "Dismiss me");
    await user.click(screen.getByRole("button", { name: "Trigger" }));
    expect(screen.getByText("Dismiss me")).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Dismiss notification" }),
    );
    expect(screen.queryByText("Dismiss me")).not.toBeInTheDocument();
  });

  it("useToast outside a provider does not throw", () => {
    function Outside() {
      const { toast } = useToast();
      return (
        <button
          type="button"
          onClick={() => {
            toast.success("no-op");
            toast.error("no-op");
          }}
        >
          test
        </button>
      );
    }
    expect(() => render(<Outside />)).not.toThrow();
  });

  it("success toast auto-dismisses after timeout", () => {
    vi.useFakeTimers();
    renderToast("success", "Fading message");

    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "Trigger" }));
    });

    expect(screen.getByText("Fading message")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(4001);
    });

    expect(screen.queryByText("Fading message")).not.toBeInTheDocument();
  });
});
