"use client";

import { Ticket } from "@/stores/ticket";
import type { TicketRelation } from "@/lib/viewModels/ticket";
import { useCurrentOrg } from "@/stores/auth";
import { cn } from "@/lib/utils";
import { GitPullRequest, Clock, Terminal } from "lucide-react";
import { useRouter } from "next/navigation";
import { useTicketPods, invalidateTicketPods } from "@/hooks/useTicketPods";
import { useWorkspaceStore } from "@/stores/workspace";
import { getShortPodKey } from "@/lib/pod-display-name";
import { AgentStatusBadge } from "@/components/shared/AgentStatusBadge";
import { SpawnPodButton } from "./SpawnPodButton";
import { RailSection, RailEmpty } from "./TicketRailSection";
import { SubTicketsRail } from "./SubTicketsRail";

interface TicketDetailSidebarProps {
  ticket: Ticket;
  ticketSlug: string;
  subTickets?: Ticket[];
  relations?: TicketRelation[];
  commits?: Array<{
    sha?: string;
    message?: string;
    hash?: string;
    short_sha?: string;
    created_at?: string;
    url?: string;
  }>;
  t: (key: string, params?: Record<string, string | number>) => string;
  commentsSlot?: React.ReactNode;
  onSubTicketCreated?: () => void;
}

export function TicketDetailSidebar({
  ticket,
  ticketSlug,
  subTickets = [],
  relations = [],
  commits = [],
  t,
  commentsSlot,
  onSubTicketCreated,
}: TicketDetailSidebarProps) {
  const router = useRouter();
  const currentOrg = useCurrentOrg();
  const addPane = useWorkspaceStore((s) => s.addPane);
  const { pods, loading: podsLoading } = useTicketPods(ticketSlug);

  const activePods = pods.filter((p) => p.status === "running" || p.status === "initializing");

  const handleOpenPod = (podKey: string) => {
    addPane(podKey);
    router.push(`/${currentOrg?.slug}/workspace`);
  };

  return (
    <aside className="lg:w-80 shrink-0 space-y-4">
      <div className="space-y-1.5">
        <SpawnPodButton
          ticket={ticket}
          ticketSlug={ticketSlug}
          onPodCreated={() => invalidateTicketPods(ticketSlug)}
          size="lg"
          className="h-11 w-full gap-2 text-sm font-semibold shadow-sm"
        />
        <p className="text-[11px] text-muted-foreground">
          {ticket.repository?.name ?? "—"} · {t("tickets.detail.lastUsedAgent")}
        </p>
      </div>

      <RailSection title={t("tickets.rail.workingPods")} count={activePods.length} data-testid="working-pods-rail">
        {podsLoading ? (
          <RailEmpty icon={<Terminal className="h-4 w-4" />} text={t("common.loading")} />
        ) : activePods.length === 0 ? (
          <RailEmpty icon={<Terminal className="h-4 w-4" />} text={t("tickets.rail.noPods")} />
        ) : (
          <ul className="space-y-1">
            {activePods.map((pod) => (
              <li key={pod.pod_key}>
                <button
                  type="button"
                  onClick={() => handleOpenPod(pod.pod_key)}
                  className="w-full rounded-md px-2 py-1.5 text-left transition-colors hover:bg-muted"
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex min-w-0 items-center gap-2">
                      <span
                        className={cn(
                          "h-2 w-2 flex-shrink-0 rounded-full",
                          pod.status === "running" ? "bg-success" : "bg-warning",
                        )}
                      />
                      <span className="truncate font-mono text-[12px] font-medium text-foreground">
                        {getShortPodKey(pod.pod_key)}
                      </span>
                    </div>
                    <AgentStatusBadge agentStatus={pod.agent_status} podStatus={pod.status} variant="badge" />
                  </div>
                </button>
              </li>
            ))}
          </ul>
        )}
      </RailSection>

      <SubTicketsRail
        parentSlug={ticketSlug}
        parentRepositoryId={ticket.repository_id}
        subTickets={subTickets}
        onCreated={() => onSubTicketCreated?.()}
        t={t}
      />

      <RailSection title={t("tickets.rail.pullRequests")} count={commits.length}>
        {commits.length === 0 ? (
          <RailEmpty icon={<GitPullRequest className="h-4 w-4" />} text={t("tickets.rail.noPRs")} />
        ) : (
          <ul className="space-y-1">
            {commits.slice(0, 5).map((c, idx) => (
              <li
                key={c.sha ?? c.hash ?? idx}
                className="flex items-start gap-2 rounded-md px-2 py-1.5 hover:bg-muted"
              >
                <GitPullRequest className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <div className="font-mono text-[10px] text-muted-foreground">
                    {(c.short_sha ?? c.sha)?.slice(0, 7)}
                  </div>
                  <div className="truncate text-[12px] text-foreground">{c.message}</div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </RailSection>

      <RailSection title={t("tickets.rail.activity")}>
        <ul className="space-y-2">
          <ActivityRow time={ticket.updated_at} text={t("tickets.rail.activityUpdated")} />
          <ActivityRow time={ticket.created_at} text={t("tickets.rail.activityCreated")} />
          {relations.slice(0, 2).map((rel, idx) => (
            <ActivityRow
              key={`rel-${idx}`}
              time={undefined}
              text={`${rel.relation_type}: ${rel.target_ticket?.slug ?? "—"}`}
            />
          ))}
        </ul>
      </RailSection>

      {commentsSlot}
    </aside>
  );
}

function formatRelative(time: string): string {
  const diffMs = Date.now() - new Date(time).getTime();
  const hours = Math.floor(diffMs / (60 * 60 * 1000));
  const days = Math.floor(hours / 24);
  return days > 0 ? `${days}d ago` : hours > 0 ? `${hours}h ago` : "just now";
}

function ActivityRow({ time, text }: { time?: string; text: string }) {
  if (!time) return null;
  const rel = formatRelative(time);
  return (
    <li className="flex items-start gap-2 px-2">
      <Clock className="mt-0.5 h-3 w-3 flex-shrink-0 text-muted-foreground/60" />
      <div className="flex-1 text-[12px]">
        <div className="text-foreground">{text}</div>
        <div className="text-[10px] text-muted-foreground">{rel}</div>
      </div>
    </li>
  );
}

export default TicketDetailSidebar;
