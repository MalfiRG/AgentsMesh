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
import { readCurrentOrg } from "@/stores/auth";

beforeEach(() => {
  __resetRepositoryBranchesCacheForTests();
  vi.mocked(listRepositoryBranches).mockReset();
  vi.mocked(readCurrentOrg).mockReturnValue({ slug: "test-org" } as ReturnType<typeof readCurrentOrg>);
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

// S4: per-repoId cache isolation is the actual guard against cross-repo stale responses.
// The hook removed the reqToken field (dead under no-op-refresh design: two requests for
// the same repoId cannot be in-flight simultaneously, and different repoIds use entirely
// separate cache entries). This test directly verifies the real protective property:
// a late response for repo A writes to cache[1], while the hook rendered with id=2 reads
// from cache[2] — the two entries are independent, so A's late resolve cannot affect B's
// rendered branches regardless of ordering.
test("stale response for repo A is dropped after switching to repo B (S4 — cache isolation)", async () => {
  const deferred: Record<number, (v: unknown) => void> = {};
  vi.mocked(listRepositoryBranches).mockImplementation((_org, id) =>
    new Promise((r) => { deferred[id as number] = r; }) as never);
  const { result, rerender } = renderHook(({ id }) => useRepositoryBranches(id), { initialProps: { id: 1 } });
  act(() => result.current.load());
  rerender({ id: 2 });
  act(() => result.current.load());
  // Resolve B first; assert B is committed.
  await act(async () => { deferred[2]({ items: ["b-branch"], total: 1, limit: 100, offset: 0 }); });
  await waitFor(() => expect(result.current.branches).toEqual(["b-branch"]));
  // Resolve A late — A writes to cache[1], hook reads cache[2], so B's state is unchanged.
  await act(async () => { deferred[1]({ items: ["a-branch"], total: 1, limit: 100, offset: 0 }); });
  expect(result.current.branches).toEqual(["b-branch"]);
});

test("fetch error (no-credential) sets fallbackToFreeText", async () => {
  vi.mocked(listRepositoryBranches).mockRejectedValue(new Error("FailedPrecondition: no git credential"));
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.fallbackToFreeText).toBe(true));
  expect(result.current.branches).toEqual([]);
});

test("fetch error (generic provider error) sets fallbackToFreeText", async () => {
  vi.mocked(listRepositoryBranches).mockRejectedValue(new Error("provider unavailable"));
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.fallbackToFreeText).toBe(true));
  expect(result.current.branches).toEqual([]);
});

test("fetch error (auth error mid-fetch) sets fallbackToFreeText", async () => {
  vi.mocked(listRepositoryBranches).mockRejectedValue(new Error("Unauthenticated"));
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.fallbackToFreeText).toBe(true));
  expect(result.current.branches).toEqual([]);
});

// X5: no-op refresh while loading means exactly ONE network call is issued.
// This is machine-checked by toHaveBeenCalledTimes(1).
test("same-repo overlap: refresh() no-ops while loading, exactly one request issued (X5)", async () => {
  const calls: Array<(v: unknown) => void> = [];
  vi.mocked(listRepositoryBranches).mockImplementation(() =>
    new Promise((r) => { calls.push(r); }) as never);
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  act(() => result.current.refresh());
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);
  await act(async () => { calls[0]({ items: ["b-branch"], total: 1, limit: 100, offset: 0 }); });
  await waitFor(() => expect(result.current.branches).toEqual(["b-branch"]));
  expect(result.current.loading).toBe(false);
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);
});

// Recovery: load() retries after error (status "error" is not in the no-op set).
// refresh() also retries after error (no-op only for status "loading").
// This is intentional: both allow recovery from a transient error.
test("load() retries after error (intentional recovery behavior)", async () => {
  vi.mocked(listRepositoryBranches)
    .mockRejectedValueOnce(new Error("transient"))
    .mockResolvedValue({ items: ["main"], total: 1, limit: 100, offset: 0 });
  const { result } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.fallbackToFreeText).toBe(true));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.branches).toEqual(["main"]));
  expect(result.current.fallbackToFreeText).toBe(false);
});

// I3: cache entry for a repoId is evicted when its last subscriber unmounts.
// After unmount, a fresh mount must re-fetch rather than serve the stale cached value.
test("cache entry is evicted after last subscriber unmounts, fresh mount re-fetches", async () => {
  vi.mocked(listRepositoryBranches).mockResolvedValue({ items: ["main"], total: 1, limit: 100, offset: 0 });
  const { result, unmount } = renderHook(() => useRepositoryBranches(1));
  act(() => result.current.load());
  await waitFor(() => expect(result.current.branches).toEqual(["main"]));
  expect(listRepositoryBranches).toHaveBeenCalledTimes(1);

  unmount();

  vi.mocked(listRepositoryBranches).mockResolvedValue({ items: ["updated"], total: 1, limit: 100, offset: 0 });
  const { result: result2 } = renderHook(() => useRepositoryBranches(1));
  act(() => result2.current.load());
  await waitFor(() => expect(result2.current.branches).toEqual(["updated"]));
  expect(listRepositoryBranches).toHaveBeenCalledTimes(2);
});
