import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { TagInput } from "./tag-input";

// ── Helpers ───────────────────────────────────────────────────────────────────

function setup(
  tags: string[] = [],
  opts: { allTags?: string[]; disabled?: boolean } = {},
) {
  const onChange = vi.fn();
  const utils = render(
    <TagInput
      tags={tags}
      allTags={opts.allTags}
      onChange={onChange}
      disabled={opts.disabled}
    />,
  );
  const input = utils.queryByRole("textbox") as HTMLInputElement | null;
  return { ...utils, input, onChange };
}

// ── Rendering ─────────────────────────────────────────────────────────────────

describe("TagInput", () => {
  describe("rendering", () => {
    it("renders existing tags", () => {
      setup(["prod", "web"]);
      expect(screen.getByText("prod")).toBeInTheDocument();
      expect(screen.getByText("web")).toBeInTheDocument();
    });

    it("renders text input when not disabled", () => {
      const { input } = setup([]);
      expect(input).toBeInTheDocument();
    });

    it("does not render text input when disabled", () => {
      const { input } = setup(["prod"], { disabled: true });
      expect(input).not.toBeInTheDocument();
    });

    it("shows placeholder when no tags", () => {
      render(<TagInput tags={[]} onChange={vi.fn()} placeholder="Add tag…" />);
      expect(screen.getByPlaceholderText("Add tag…")).toBeInTheDocument();
    });

    it("hides placeholder when tags exist", () => {
      const { input } = setup(["prod"]);
      expect(input?.placeholder).toBe("");
    });

    it("renders remove button for each tag when not disabled", () => {
      setup(["prod", "web"]);
      expect(screen.getByLabelText('Remove tag "prod"')).toBeInTheDocument();
      expect(screen.getByLabelText('Remove tag "web"')).toBeInTheDocument();
    });

    it("does not render remove buttons when disabled", () => {
      setup(["prod"], { disabled: true });
      expect(screen.queryByLabelText('Remove tag "prod"')).not.toBeInTheDocument();
    });
  });

  // ── Tag addition ─────────────────────────────────────────────────────────────

  describe("adding tags", () => {
    it("adds tag on Enter", async () => {
      const { input, onChange } = setup([]);
      await userEvent.type(input!, "newtag{Enter}");
      expect(onChange).toHaveBeenCalledWith(["newtag"]);
    });

    it("adds tag on comma", async () => {
      const { input, onChange } = setup([]);
      await userEvent.type(input!, "newtag,");
      expect(onChange).toHaveBeenCalledWith(["newtag"]);
    });

    it("trims whitespace and lowercases the tag", async () => {
      const { input, onChange } = setup([]);
      await userEvent.type(input!, "  MyTag  {Enter}");
      expect(onChange).toHaveBeenCalledWith(["mytag"]);
    });

    it("ignores empty/whitespace-only input on Enter", async () => {
      const { input, onChange } = setup([]);
      await userEvent.type(input!, "   {Enter}");
      expect(onChange).not.toHaveBeenCalled();
    });

    it("does not add duplicate tag", async () => {
      const { input, onChange } = setup(["prod"]);
      await userEvent.type(input!, "prod{Enter}");
      expect(onChange).not.toHaveBeenCalled();
    });

    it("appends to existing tags", async () => {
      const { input, onChange } = setup(["prod"]);
      await userEvent.type(input!, "web{Enter}");
      expect(onChange).toHaveBeenCalledWith(["prod", "web"]);
    });
  });

  // ── Tag removal ──────────────────────────────────────────────────────────────

  describe("removing tags", () => {
    it("removes tag via remove button click", async () => {
      const { onChange } = setup(["prod", "web"]);
      await userEvent.click(screen.getByLabelText('Remove tag "prod"'));
      expect(onChange).toHaveBeenCalledWith(["web"]);
    });

    it("removes last tag on Backspace with empty input", async () => {
      const { input, onChange } = setup(["prod", "web"]);
      await userEvent.type(input!, "{Backspace}");
      expect(onChange).toHaveBeenCalledWith(["prod"]);
    });

    it("does not remove on Backspace when input has text", async () => {
      const { input, onChange } = setup(["prod"]);
      await userEvent.type(input!, "ab{Backspace}");
      expect(onChange).not.toHaveBeenCalled();
    });

    it("does not remove on Backspace when no tags", async () => {
      const { input, onChange } = setup([]);
      await userEvent.type(input!, "{Backspace}");
      expect(onChange).not.toHaveBeenCalled();
    });
  });

  // ── Dropdown visibility ───────────────────────────────────────────────────────

  describe("dropdown", () => {
    it("shows suggestions matching input", async () => {
      const { input } = setup([], { allTags: ["production", "staging", "web"] });
      await userEvent.type(input!, "pro");
      expect(screen.getByText("production")).toBeInTheDocument();
      expect(screen.queryByText("staging")).not.toBeInTheDocument();
    });

    it("hides already-applied tags from suggestions", async () => {
      const { input } = setup(["prod"], { allTags: ["prod", "web"] });
      await userEvent.type(input!, "p");
      // "prod" is applied — it must not appear as a suggestion option
      const options = screen.queryAllByRole("option");
      const optionTexts = options.map((o) => o.textContent);
      expect(optionTexts.some((t) => t === "prod")).toBe(false);
    });

    it("shows 'Create' option for new tag input", async () => {
      const { input } = setup([], { allTags: ["existing"] });
      await userEvent.type(input!, "newone");
      expect(screen.getByText("Create")).toBeInTheDocument();
      expect(screen.getByText('"newone"')).toBeInTheDocument();
    });

    it("hides 'Create' option when input matches existing tag exactly", async () => {
      const { input } = setup(["web"], { allTags: ["web", "prod"] });
      await userEvent.type(input!, "web");
      expect(screen.queryByText("Create")).not.toBeInTheDocument();
    });

    it("closes dropdown on Escape", async () => {
      const { input } = setup([], { allTags: ["prod"] });
      await userEvent.type(input!, "p");
      expect(screen.getByText("prod")).toBeInTheDocument();
      await userEvent.keyboard("{Escape}");
      expect(screen.queryByText("prod")).not.toBeInTheDocument();
    });

    it("adds tag when suggestion clicked", async () => {
      const { input, onChange } = setup([], { allTags: ["production"] });
      await userEvent.type(input!, "pro");
      fireEvent.mouseDown(screen.getByText("production"));
      expect(onChange).toHaveBeenCalledWith(["production"]);
    });
  });

  // ── Arrow key navigation ──────────────────────────────────────────────────────

  describe("arrow key navigation", () => {
    it("opens dropdown and highlights first item on ArrowDown", async () => {
      const { input } = setup([], { allTags: ["alpha", "beta"] });
      await userEvent.type(input!, "a");
      await userEvent.keyboard("{ArrowDown}");
      const options = screen.getAllByRole("option");
      expect(options[0]).toHaveAttribute("aria-selected", "true");
    });

    it("moves highlight down on repeated ArrowDown", async () => {
      const { input } = setup([], { allTags: ["alpha", "beta"] });
      await userEvent.type(input!, "a");
      await userEvent.keyboard("{ArrowDown}{ArrowDown}");
      const options = screen.getAllByRole("option");
      expect(options[0]).toHaveAttribute("aria-selected", "false");
      expect(options[1]).toHaveAttribute("aria-selected", "true");
    });

    it("does not move beyond the last item on ArrowDown", async () => {
      const { input } = setup([], { allTags: ["only"] });
      await userEvent.type(input!, "o");
      // Dropdown has: [Create "o", "only"] — 2 items, indices 0 and 1.
      // After 5 ArrowDowns the index should be clamped at 1.
      await userEvent.keyboard(
        "{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}",
      );
      const options = screen.getAllByRole("option");
      expect(options[options.length - 1]).toHaveAttribute(
        "aria-selected",
        "true",
      );
    });

    it("moves highlight up on ArrowUp", async () => {
      const { input } = setup([], { allTags: ["alpha", "beta"] });
      await userEvent.type(input!, "a");
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{ArrowUp}");
      const options = screen.getAllByRole("option");
      expect(options[0]).toHaveAttribute("aria-selected", "true");
      expect(options[1]).toHaveAttribute("aria-selected", "false");
    });

    it("ArrowUp from index 0 deselects all (index -1)", async () => {
      const { input } = setup([], { allTags: ["alpha"] });
      await userEvent.type(input!, "a");
      await userEvent.keyboard("{ArrowDown}{ArrowUp}");
      const options = screen.getAllByRole("option");
      expect(options[0]).toHaveAttribute("aria-selected", "false");
    });

    it("Enter selects highlighted item", async () => {
      // Use an exact match input so there's no "Create" option — only "alpha".
      const { input, onChange } = setup([], { allTags: ["alpha"] });
      await userEvent.type(input!, "alpha");
      // No Create option when input exactly matches a suggestion that isn't applied.
      // Dropdown: [index 0 = "alpha"].
      await userEvent.keyboard("{ArrowDown}{Enter}");
      expect(onChange).toHaveBeenCalledWith(["alpha"]);
    });

    it("Enter with no highlighted item adds typed input", async () => {
      const { input, onChange } = setup([], { allTags: ["alpha"] });
      await userEvent.type(input!, "custom{Enter}");
      expect(onChange).toHaveBeenCalledWith(["custom"]);
    });

    it("Escape clears highlight and closes dropdown", async () => {
      const { input } = setup([], { allTags: ["alpha"] });
      await userEvent.type(input!, "a");
      await userEvent.keyboard("{ArrowDown}{Escape}");
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    });

    it("resets highlight when input changes", async () => {
      const { input } = setup([], { allTags: ["alpha", "beta"] });
      await userEvent.type(input!, "a");
      await userEvent.keyboard("{ArrowDown}");
      // Type another character — highlight should reset
      await userEvent.type(input!, "l");
      const options = screen.getAllByRole("option");
      options.forEach((opt) =>
        expect(opt).toHaveAttribute("aria-selected", "false"),
      );
    });

    it("hovering an item updates the active index", async () => {
      const { input } = setup([], { allTags: ["alpha", "beta"] });
      await userEvent.type(input!, "a");
      const options = screen.getAllByRole("option");
      fireEvent.mouseEnter(options[1]!);
      expect(options[1]).toHaveAttribute("aria-selected", "true");
      expect(options[0]).toHaveAttribute("aria-selected", "false");
    });
  });

  // ── Accessibility ─────────────────────────────────────────────────────────────

  describe("accessibility", () => {
    it("dropdown has listbox role", async () => {
      const { input } = setup([], { allTags: ["prod"] });
      await userEvent.type(input!, "p");
      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });

    it("each dropdown item has option role", async () => {
      const { input } = setup([], { allTags: ["prod", "staging"] });
      await userEvent.type(input!, "p");
      const options = screen.getAllByRole("option");
      expect(options.length).toBeGreaterThan(0);
    });

    it("remove buttons have accessible labels", () => {
      setup(["prod", "web"]);
      expect(screen.getByLabelText('Remove tag "prod"')).toBeInTheDocument();
      expect(screen.getByLabelText('Remove tag "web"')).toBeInTheDocument();
    });
  });
});
