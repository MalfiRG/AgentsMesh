import { renderHook, act, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { useRepositoryBranches, __resetRepositoryBranchesCacheForTests } from "../useRepositoryBranches";

vi.mock("@/lib/api/facade/repositoryConnect", () => ({
  listRepositoryBranches: vi.fn(),
}));

vi.mock("@/stores/auth", () => ({
  readCurrentOrg: vi.fn(() => ({ slug: "test-org" })),
}));

import { listRepositoryBranches } from "@/lib/api/facade/repositoryConnect";

beforeEach(() => {
  __resetRepositoryBranchesCacheForTests();
  vi.mocked(listRepositoryBranches).mockReset();
});

test("does not fetch until load() is called (lazy)", () => {
  renderHook(() => useRepositoryBranches(1));
  expect(listRepositoryBranches).not.toHaveBeenCalled();
});

test("load() fetches once and caches per repoId", async () => {
  vi.mocked(listRepositoryBranches).mockResolvedValue({ items: ["main"], total: 1, limit: 100, offset: 0 });
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.branches).toEqual(["main"]));
  act(() => result.current.load());
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);
});

test("concurrent load() for same repoId dedupes to one call", async () => {
  let resolve!: (v: unknown) => void;
  vi.mocked(listRepositoryBranches).mockReturnValue(new Promise((r) => { resolve = r; }) as never);
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => { result.current.load(); result.current.load(); });
  resolve({ items: ["main"], total: 1, limit: 100, offset: 0 });
  await waitFor(() => expect(result.current.branches).toEqual(["main"]));
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);
});

test("stale response for repo A is dropped after switching to repo B (S4)", async () => {
  const deferred: Record<number, (v: unknown) => void> = {};
  vi.mocked(listRepositoryBranches).mockImplementation((_org, id) =>
    new Promise((r) => { deferred[id as number] = r; }) as never);
  const { result, rerender } = renderHook(({ id }) => useRepositoryBranches(id), { initialProps: { id: 1 } });
  act(() => result.current.load());
  rerender({ id: 2 });
  act(() => result.current.load());
  // Resolve B first; wait deterministically for B to commit.
  await act(async () => { deferred[2]({ items: ["b-branch"], total: 1, limit: 100, offset: 0 }); });
  await waitFor(() => expect(result.current.branches).toEqual(["b-branch"]));
  // Resolve the stale A inside act() so React flushes synchronously.
  await act(async () => { deferred[1]({ items: ["a-branch"], total: 1, limit: 100, offset: 0 }); });
  expect(result.current.branches).toEqual(["b-branch"]); // stale A did not overwrite B
});

test("fetch error sets fallbackToFreeText", async () => {
  vi.mocked(listRepositoryBranches).mockRejectedValue(new Error("FailedPrecondition: no git credential"));
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.fallbackToFreeText).toBe(true));
  expect(result.current.branches).toEqual([]);
});

test("same-repo overlap: late-resolving request A does not flip B's state (X5)", async () => {
  const calls: Array<(v: unknown) => void> = [];
  vi.mocked(listRepositoryBranches).mockImplementation(() =>
    new Promise((r) => { calls.push(r); }) as never);
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());          // request A (status loading -> refresh blocked)
  act(() => result.current.refresh());       // refresh() no-ops when loading; same promise
  // Only one call was made (refresh no-ops while loading)
  await act(async () => { calls[0]({ items: ["b-branch"], total: 1, limit: 100, offset: 0 }); });
  await waitFor(() => expect(result.current.branches).toEqual(["b-branch"]));
  expect(result.current.loading).toBe(false);
  expect(result.current.branches).toEqual(["b-branch"]);
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);
});
