import { renderHook } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { Connection } from "@xyflow/react";

const mockRequestBindingConnect = vi.fn();
const mockFetchTopology = vi.fn();
const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
let currentOrgSlug: string | undefined = "test-org";

vi.mock("@/stores/auth", () => ({
  useCurrentOrg: () => (currentOrgSlug ? { slug: currentOrgSlug } : undefined),
}));

vi.mock("@/stores/mesh", () => ({
  useMeshStore: (selector: (s: { fetchTopology: () => void }) => unknown) =>
    selector({ fetchTopology: mockFetchTopology }),
}));

vi.mock("@/lib/api/facade/bindingConnect", () => ({
  requestBindingConnect: (...args: unknown[]) => mockRequestBindingConnect(...args),
}));

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock("@/lib/api/errors", () => ({
  getLocalizedErrorMessage: (_e: unknown, _t: unknown, fallback: string) => fallback,
}));

vi.mock("sonner", () => ({
  toast: { success: (m: string) => mockToastSuccess(m), error: (m: string) => mockToastError(m) },
}));

import { useMeshConnect } from "../useMeshConnect";

const conn = (source: string, target: string): Connection => ({
  source,
  target,
  sourceHandle: null,
  targetHandle: null,
});

beforeEach(() => {
  vi.clearAllMocks();
  currentOrgSlug = "test-org";
  mockRequestBindingConnect.mockResolvedValue({});
});

describe("useMeshConnect", () => {
  it("requests a read+write binding with same_user_auto, then refetches and toasts success", async () => {
    const { result } = renderHook(() => useMeshConnect());
    await result.current(conn("pod-a", "pod-b"));

    expect(mockRequestBindingConnect).toHaveBeenCalledWith(
      "test-org",
      "pod-a",
      "pod-b",
      ["pod:read", "pod:write"],
      "same_user_auto",
    );
    expect(mockFetchTopology).toHaveBeenCalledTimes(1);
    expect(mockToastSuccess).toHaveBeenCalledTimes(1);
    expect(mockToastError).not.toHaveBeenCalled();
  });

  it("no-ops on a self-connection", async () => {
    const { result } = renderHook(() => useMeshConnect());
    await result.current(conn("pod-a", "pod-a"));
    expect(mockRequestBindingConnect).not.toHaveBeenCalled();
  });

  it("no-ops when there is no current org", async () => {
    currentOrgSlug = undefined;
    const { result } = renderHook(() => useMeshConnect());
    await result.current(conn("pod-a", "pod-b"));
    expect(mockRequestBindingConnect).not.toHaveBeenCalled();
  });

  it("shows an error toast and does not refetch when the request fails", async () => {
    mockRequestBindingConnect.mockRejectedValue(new Error("already bound"));
    const { result } = renderHook(() => useMeshConnect());
    await result.current(conn("pod-a", "pod-b"));

    expect(mockToastError).toHaveBeenCalledTimes(1);
    expect(mockFetchTopology).not.toHaveBeenCalled();
    expect(mockToastSuccess).not.toHaveBeenCalled();
  });
});
