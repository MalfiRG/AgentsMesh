import { describe, it, expect, vi, beforeEach } from "vitest";
import { handlePodEvent } from "../realtimePodHandlers";
import type { RealtimeEvent } from "@/lib/realtime";

const mockInvalidate = vi.fn();
vi.mock("@/hooks/useTicketPods", () => ({
  invalidateTicketPods: (s: string) => mockInvalidate(s),
}));

const mockFetchSidebarPods = vi.fn();
const mockFetchTopology = vi.fn();
const mockFetchPod = vi.fn();
const mockRemovePaneByPodKey = vi.fn();

let podTick = 0;
let meshTick = 0;

vi.mock("@/stores/pod", () => ({
  usePodStore: {
    getState: () => ({
      currentSidebarFilter: undefined,
      fetchSidebarPods: mockFetchSidebarPods,
      fetchPod: mockFetchPod,
      _tick: podTick,
    }),
    setState: (updater: (s: { _tick: number }) => unknown) => {
      const next = updater({ _tick: podTick }) as { _tick: number };
      podTick = next._tick;
    },
  },
}));

vi.mock("@/stores/mesh", () => ({
  useMeshStore: {
    getState: () => ({
      fetchTopology: mockFetchTopology,
      _tick: meshTick,
    }),
    setState: (updater: (s: { _tick: number }) => unknown) => {
      const next = updater({ _tick: meshTick }) as { _tick: number };
      meshTick = next._tick;
    },
  },
}));

vi.mock("@/stores/workspace", () => ({
  useWorkspaceStore: {
    getState: () => ({
      removePaneByPodKey: mockRemovePaneByPodKey,
    }),
  },
}));

vi.mock("@/lib/wasm-core", () => ({
  getPodState: () => ({
    get_pod_json: (_key: string) => null,
  }),
}));

const baseFields = {
  category: "entity" as const,
  organization_id: 1,
  entity_type: "pod",
  entity_id: "p1",
  timestamp: Date.now(),
};

function makePodCreatedEvent({ podKey, ticketSlug }: { podKey: string; ticketSlug: string }): RealtimeEvent {
  return {
    type: "pod:created",
    data: { pod_key: podKey, ticket_slug: ticketSlug },
    ...baseFields,
  };
}

function makePodStatusChangedEvent({ podKey }: { podKey: string }): RealtimeEvent {
  return {
    type: "pod:status_changed",
    data: { pod_key: podKey, status: "running", agent_status: "" },
    ...baseFields,
  };
}

describe("handlePodEvent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    podTick = 0;
    meshTick = 0;
  });

  describe("pod:created", () => {
    it("invalidates that ticket when ticketSlug is present", () => {
      handlePodEvent(makePodCreatedEvent({ podKey: "p1", ticketSlug: "AM-7" }));
      expect(mockInvalidate).toHaveBeenCalledWith("AM-7");
    });

    it("does NOT invalidate when ticketSlug is empty", () => {
      handlePodEvent(makePodCreatedEvent({ podKey: "p1", ticketSlug: "" }));
      expect(mockInvalidate).not.toHaveBeenCalled();
    });

    it("calls refreshSidebar (fetchSidebarPods) and refreshMeshTopology (fetchTopology)", () => {
      handlePodEvent(makePodCreatedEvent({ podKey: "p1", ticketSlug: "" }));
      expect(mockFetchSidebarPods).toHaveBeenCalled();
      expect(mockFetchTopology).toHaveBeenCalled();
    });
  });

  describe("pod:restarting", () => {
    it("calls refreshSidebar and refreshMeshTopology but does NOT invalidate ticket", () => {
      handlePodEvent({ type: "pod:restarting", data: { pod_key: "p1" }, ...baseFields });
      expect(mockFetchSidebarPods).toHaveBeenCalled();
      expect(mockFetchTopology).toHaveBeenCalled();
      expect(mockInvalidate).not.toHaveBeenCalled();
    });
  });

  describe("pod:status_changed", () => {
    it("does NOT invalidate ticket pods (never calls invalidateTicketPods)", () => {
      handlePodEvent(makePodStatusChangedEvent({ podKey: "p1" }));
      expect(mockInvalidate).not.toHaveBeenCalled();
    });

    it("does NOT call refreshSidebar or refreshMeshTopology (only bumps ticks / fetches pod)", () => {
      handlePodEvent(makePodStatusChangedEvent({ podKey: "p1" }));
      expect(mockFetchSidebarPods).not.toHaveBeenCalled();
      expect(mockFetchTopology).not.toHaveBeenCalled();
    });

    it("fetches the pod when not cached, and does not call removePaneByPodKey for non-terminal status", () => {
      handlePodEvent(makePodStatusChangedEvent({ podKey: "p1" }));
      expect(mockFetchPod).toHaveBeenCalledWith("p1");
      expect(mockRemovePaneByPodKey).not.toHaveBeenCalled();
    });
  });
});
