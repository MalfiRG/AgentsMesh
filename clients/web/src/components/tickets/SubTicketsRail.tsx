"use client";

import { useState } from "react";
import Link from "next/link";
import { CheckCircle2, Circle, Plus } from "lucide-react";
import { Ticket } from "@/stores/ticket";
import { useCurrentOrg } from "@/stores/auth";
import { cn } from "@/lib/utils";
import { RailSection, RailEmpty } from "./TicketRailSection";
import { TicketCreateDialog } from "./TicketCreateDialog";

interface SubTicketsRailProps {
  parentSlug: string;
  parentRepositoryId?: number | null;
  subTickets: Ticket[];
  onCreated: () => void;
  t: (key: string) => string;
}

export function SubTicketsRail({
  parentSlug,
  parentRepositoryId,
  subTickets,
  onCreated,
  t,
}: SubTicketsRailProps) {
  const currentOrg = useCurrentOrg();
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <>
      <RailSection
        title={t("tickets.rail.subTickets")}
        count={subTickets.length}
        action={
          <button
            type="button"
            onClick={() => setDialogOpen(true)}
            aria-label={t("tickets.rail.addSubTicket")}
            title={t("tickets.rail.addSubTicket")}
            className="rounded p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        }
      >
        {subTickets.length === 0 ? (
          <RailEmpty icon={<Circle className="h-4 w-4" />} text={t("tickets.rail.noSubTickets")} />
        ) : (
          <ul className="space-y-1">
            {subTickets.map((st) => {
              const isDone = st.status === "done";
              return (
                <li key={st.slug}>
                  <Link
                    href={`/${currentOrg?.slug}/tickets/${st.slug}`}
                    className="flex items-start gap-2 rounded-md px-2 py-1.5 hover:bg-muted"
                  >
                    {isDone ? (
                      <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-success" />
                    ) : (
                      <Circle className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
                    )}
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <span className="font-mono text-[10px] text-muted-foreground">{st.slug}</span>
                      </div>
                      <div
                        className={cn(
                          "truncate text-[12px]",
                          isDone ? "text-muted-foreground line-through" : "text-foreground",
                        )}
                      >
                        {st.title}
                      </div>
                    </div>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </RailSection>

      <TicketCreateDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        parentTicketSlug={parentSlug}
        defaultRepositoryId={parentRepositoryId ?? null}
        onCreated={() => onCreated()}
      />
    </>
  );
}

export default SubTicketsRail;
