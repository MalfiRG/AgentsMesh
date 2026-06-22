import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@/test/test-utils";
import { create, toBinary } from "@bufbuild/protobuf";
import { ReplaceCachedPodsRequestSchema } from "@proto/pod_state/v1/pod_state_pb";
import { PodSchema } from "@proto/pod/v1/pod_pb";

import { TicketDetailSidebar } from "../TicketDetailSidebar";
import { __resetTicketPodsCacheForTests } from "@/hooks/useTicketPods";

const stateMirror = new Map<string, string>();
const getTicketPodsMock = vi.fn(async (slug: string) =>
  JSON.stringify({ pods: [] }),
);

vi.mock("@/lib/wasm-core", async () => {
  const actual = await vi.importActual<typeof import("@/lib/wasm-core")>(
    "@/lib/wasm-core",
  );
  return {
    ...actual,
    getTicketService: () => ({
      get_ticket_pods: getTicketPodsMock,
    }),
    getTicketState: () => ({
      ticket_pods_bytes: (slug: string) => {
        const pods = JSON.parse(stateMirror.get(slug) ?? "[]") as {
          pod_key: string;
          status?: string;
        }[];
        return toBinary(
          ReplaceCachedPodsRequestSchema,
          create(ReplaceCachedPodsRequestSchema, {
            pods: pods.map((p) =>
              create(PodSchema, { podKey: p.pod_key, status: p.status ?? "" }),
            ),
          }),
        );
      },
      set_ticket_pods: (slug: string, podsJson: string) => {
        stateMirror.set(slug, podsJson);
      },
    }),
  };
});

vi.mock("../SpawnPodButton", () => ({
  SpawnPodButton: ({
    onPodCreated,
  }: {
    onPodCreated?: () => void;
    ticket?: unknown;
    ticketSlug?: string;
    size?: string;
    className?: string;
  }) => <button onClick={onPodCreated}>spawn</button>,
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@/stores/auth", () => ({
  useCurrentOrg: () => ({ id: 1, slug: "test-org" }),
}));

vi.mock("@/stores/workspace", () => ({
  useWorkspaceStore: () => ({ addPane: vi.fn() }),
}));

vi.mock("@/lib/pod-display-name", () => ({
  getShortPodKey: (key: string) => key,
}));

vi.mock("@/components/shared/AgentStatusBadge", () => ({
  AgentStatusBadge: () => null,
}));

vi.mock("@/components/tickets/SubTicketsRail", () => ({
  SubTicketsRail: () => null,
}));

const baseTicket = {
  id: 1,
  number: 1,
  slug: "AM-1",
  title: "Test ticket",
  content: "",
  status: "todo" as const,
  priority: "medium" as const,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  organization_id: 1,
  repository_id: null,
  repository: null,
  assignees: [],
  labels: [],
};

const t = (key: string) => key;

describe("TicketDetailSidebar pod refresh", () => {
  beforeEach(() => {
    stateMirror.clear();
    getTicketPodsMock.mockClear();
    __resetTicketPodsCacheForTests();
  });

  afterEach(() => {
    __resetTicketPodsCacheForTests();
    stateMirror.clear();
  });

  it("spawning a pod refetches this ticket's pods", async () => {
    getTicketPodsMock.mockResolvedValue(JSON.stringify({ pods: [] }));

    render(
      <TicketDetailSidebar ticket={baseTicket} ticketSlug="AM-1" t={t} />,
    );

    await waitFor(() =>
      expect(getTicketPodsMock).toHaveBeenCalledWith("AM-1", true),
    );
    getTicketPodsMock.mockClear();

    fireEvent.click(screen.getByText("spawn"));

    await waitFor(() =>
      expect(getTicketPodsMock).toHaveBeenCalledWith("AM-1", true),
    );
  });
});
