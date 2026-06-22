import { describe, test, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { BranchCombobox } from "../BranchCombobox";

const hook = {
  branches: ["main", "develop"],
  loading: false,
  fallbackToFreeText: false,
  load: vi.fn(),
  refresh: vi.fn(),
};

vi.mock("@/hooks/useRepositoryBranches", () => ({
  useRepositoryBranches: () => hook,
}));

const t = (k: string) => k;

describe("BranchCombobox", () => {
  beforeEach(() => {
    hook.branches = ["main", "develop"];
    hook.loading = false;
    hook.fallbackToFreeText = false;
    hook.load.mockClear();
    hook.refresh.mockClear();
  });

  test("opening the dropdown calls load()", () => {
    render(<BranchCombobox repoId={1} value="" onChange={() => {}} t={t} />);
    fireEvent.focus(screen.getByRole("combobox"));
    expect(hook.load).toHaveBeenCalled();
  });

  test("lists fetched branches and selecting one fires onChange", () => {
    const onChange = vi.fn();
    render(<BranchCombobox repoId={1} value="" onChange={onChange} t={t} />);
    fireEvent.focus(screen.getByRole("combobox"));
    fireEvent.click(screen.getByText("develop"));
    expect(onChange).toHaveBeenCalledWith("develop");
  });

  test("typed non-listed value is accepted (editable combobox)", () => {
    const onChange = vi.fn();
    render(<BranchCombobox repoId={1} value="" onChange={onChange} t={t} />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "feature/new" } });
    expect(onChange).toHaveBeenCalledWith("feature/new");
  });

  test("filters list as user types", () => {
    render(<BranchCombobox repoId={1} value="dev" onChange={() => {}} t={t} />);
    fireEvent.focus(screen.getByRole("combobox"));
    expect(screen.getByText("develop")).toBeInTheDocument();
    expect(screen.queryByText("main")).not.toBeInTheDocument();
  });

  test("falls back to plain text input when hook signals error", () => {
    hook.fallbackToFreeText = true;
    render(<BranchCombobox repoId={1} value="x" onChange={() => {}} t={t} />);
    expect(screen.getByLabelText(/branch/i)).toBeInTheDocument();
  });

  test("empty branch list stays editable", () => {
    hook.branches = [];
    const onChange = vi.fn();
    render(<BranchCombobox repoId={1} value="" onChange={onChange} t={t} />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "new-branch" } });
    expect(onChange).toHaveBeenCalledWith("new-branch");
  });

  test("long, slashy and unicode branch names render", () => {
    hook.branches = [
      "feature/long-branch-name-with-many-segments/sub",
      "feat/unicode-日本語",
      "fix/special_chars-and.dots",
    ];
    render(<BranchCombobox repoId={1} value="" onChange={() => {}} t={t} />);
    fireEvent.focus(screen.getByRole("combobox"));
    expect(screen.getByText("feature/long-branch-name-with-many-segments/sub")).toBeInTheDocument();
    expect(screen.getByText("feat/unicode-日本語")).toBeInTheDocument();
    expect(screen.getByText("fix/special_chars-and.dots")).toBeInTheDocument();
  });
});
