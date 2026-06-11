"use client";

import { useEffect, useCallback, useRef, lazy, Suspense, useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { ConfirmDialog, useConfirmDialog } from "@/components/ui/confirm-dialog";
import { useCurrentOrg, useAuthStore } from "@/stores/auth";
import { useTicketStore, useCurrentTicket, TicketStatus, TicketPriority } from "@/stores/ticket";
import { useTicketExtraData } from "./hooks";
import { LabelsList, CommentsList, SubTicketsList, RelationsList, CommitsList } from "./shared";
import { TicketDetailSidebar } from "./TicketDetailSidebar";
import { TicketHeaderActions } from "./TicketHeaderActions";
import { InlineEditableText, InlineEditableTextHandle } from "./InlineEditableText";
import { StatusSelect } from "./StatusSelect";
import { PrioritySelect } from "./PrioritySelect";
import { RepositoryInlineSelect } from "./RepositoryInlineSelect";
import { TicketDetailSkeleton } from "./TicketDetailSkeleton";

const BlockEditor = lazy(() => import("@/components/ui/block-editor"));

interface TicketDetailProps {
  slug: string;
}

export function TicketDetail({ slug }: TicketDetailProps) {
  const router = useRouter();
  const t = useTranslations();
  const currentOrg = useCurrentOrg();

  const currentTicket = useCurrentTicket();
  const fetchTicket = useTicketStore(state => state.fetchTicket);
  const updateTicket = useTicketStore(state => state.updateTicket);
  const updateTicketStatus = useTicketStore(state => state.updateTicketStatus);
  const deleteTicket = useTicketStore(state => state.deleteTicket);
  const setCurrentTicket = useTicketStore(state => state.setCurrentTicket);
  const error = useTicketStore(state => state.error);

  const [loadedSlug, setLoadedSlug] = useState<string | null>(null);
  const initialLoading = loadedSlug !== slug;

  const { dialogProps, confirm } = useConfirmDialog();
  const { subTickets, relations, commits, comments, refetch, addComment, updateComment, deleteComment } = useTicketExtraData(slug, !!currentTicket);

  const contentSaveTimerRef = useRef<NodeJS.Timeout | null>(null);
  const titleEditRef = useRef<InlineEditableTextHandle>(null);

  useEffect(() => {
    return () => {
      if (contentSaveTimerRef.current) {
        clearTimeout(contentSaveTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    setCurrentTicket(null);
    fetchTicket(slug).finally(() => setLoadedSlug(slug));
  }, [slug, fetchTicket, setCurrentTicket]);

  const handleTitleSave = useCallback(async (newTitle: string) => {
    if (!newTitle.trim()) return;
    try {
      await updateTicket(slug, { title: newTitle });
    } catch (err) {
      console.error("Failed to update title:", err);
      throw err;
    }
  }, [slug, updateTicket]);

  const handleContentChange = useCallback((newContent: string) => {
    if (contentSaveTimerRef.current) {
      clearTimeout(contentSaveTimerRef.current);
    }
    contentSaveTimerRef.current = setTimeout(async () => {
      try {
        await updateTicket(slug, { content: newContent });
      } catch (err) {
        console.error("Failed to update content:", err);
      }
    }, 800);
  }, [slug, updateTicket]);

  const handleStatusChange = async (newStatus: TicketStatus) => {
    try {
      await updateTicketStatus(slug, newStatus);
    } catch (err) {
      console.error("Failed to update status:", err);
    }
  };

  const handlePriorityChange = async (newPriority: TicketPriority) => {
    try {
      await updateTicket(slug, { priority: newPriority });
    } catch (err) {
      console.error("Failed to update priority:", err);
    }
  };

  const handleRepositoryChange = async (repositoryId: number | null) => {
    try {
      await updateTicket(slug, { repositoryId });
    } catch (err) {
      console.error("Failed to update repository:", err);
    }
  };

  const handleDelete = useCallback(async () => {
    const confirmed = await confirm({
      title: t("tickets.detail.deleteTicket"),
      description: t("tickets.detail.deleteConfirmation", { slug }),
      variant: "destructive",
      confirmText: t("common.delete"),
      cancelText: t("common.cancel"),
    });
    if (confirmed) {
      try {
        await deleteTicket(slug);
        router.push(`/${currentOrg?.slug}/tickets`);
      } catch (err) {
        console.error("Failed to delete ticket:", err);
      }
    }
  }, [confirm, deleteTicket, slug, router, currentOrg, t]);

  if (initialLoading && !currentTicket) {
    return <TicketDetailSkeleton />;
  }

  if (error) {
    return (
      <div className="text-center py-16">
        <div className="text-destructive mb-4 text-sm">{error}</div>
        <Button variant="outline" size="sm" onClick={() => fetchTicket(slug)}>
          {t("tickets.detail.retry")}
        </Button>
      </div>
    );
  }

  if (!currentTicket) {
    return (
      <div className="text-center py-16 text-muted-foreground text-sm">
        {t("tickets.detail.notFound")}
      </div>
    );
  }

  return (
    <div className="flex flex-col lg:flex-row gap-6 lg:gap-8">
      <div className="flex-1 min-w-0 space-y-6">
        <div className="border-b border-border pb-6">
          <div className="flex items-start justify-between gap-6">
            <div className="flex-1 min-w-0 space-y-3">
              <div className="flex items-center gap-2.5">
                <span className="font-mono text-[13px] text-muted-foreground">{slug}</span>
                <StatusSelect value={currentTicket.status} onChange={handleStatusChange} showLabel size="sm" />
              </div>
              <InlineEditableText
                ref={titleEditRef}
                value={currentTicket.title}
                onSave={handleTitleSave}
                placeholder={t("tickets.createDialog.titlePlaceholder")}
                className="text-[22px] font-semibold leading-7 text-foreground"
                inputClassName="text-[22px] font-semibold"
              />

              <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs">
                <div className="flex items-center gap-1.5">
                  <span className="text-muted-foreground">{t("tickets.detail.repository")}</span>
                  <RepositoryInlineSelect
                    value={currentTicket.repository_id ?? null}
                    currentName={currentTicket.repository?.name}
                    onChange={handleRepositoryChange}
                    size="sm"
                  />
                </div>
                <div className="flex items-center gap-1.5">
                  <span className="text-muted-foreground">{t("tickets.detail.priority")}</span>
                  <PrioritySelect
                    value={currentTicket.priority || "none"}
                    onChange={handlePriorityChange}
                    showLabel
                    size="sm"
                  />
                </div>
                {currentTicket.due_date && (
                  <div className="flex items-center gap-1.5">
                    <span className="text-muted-foreground">{t("tickets.detail.dueDate")}</span>
                    <span className="font-medium text-foreground">
                      {new Date(currentTicket.due_date).toLocaleDateString()}
                    </span>
                  </div>
                )}
                {currentTicket.created_at && (
                  <div className="flex items-center gap-1.5">
                    <span className="text-muted-foreground">{t("tickets.detail.opened")}</span>
                    <span className="text-foreground">
                      {new Date(currentTicket.created_at).toLocaleDateString()}
                    </span>
                  </div>
                )}
              </div>

              {currentTicket.labels && currentTicket.labels.length > 0 && (
                <LabelsList labels={currentTicket.labels} compact />
              )}
            </div>

            <TicketHeaderActions
              onEdit={() => titleEditRef.current?.startEditing()}
              onMarkDone={() => handleStatusChange("done" as TicketStatus)}
              onDelete={handleDelete}
            />
          </div>
        </div>

        <div className="rounded-md border border-border overflow-hidden bg-card shadow-xs min-h-[200px] max-h-[65vh] overflow-y-auto">
          <Suspense fallback={<div className="h-[200px] animate-pulse bg-muted/30 rounded-md" />}>
            <BlockEditor
              key={slug}
              initialContent={currentTicket.content || ""}
              onChange={handleContentChange}
              editable={true}
            />
          </Suspense>
        </div>

        <div className="hidden lg:block">
          <CommentsList
            comments={comments}
            onAddComment={addComment}
            onUpdateComment={updateComment}
            onDeleteComment={deleteComment}
          />
        </div>

      </div>

      <TicketDetailSidebar
        ticket={currentTicket}
        ticketSlug={slug}
        subTickets={subTickets}
        relations={relations}
        commits={commits}
        t={t}
        onSubTicketCreated={refetch}
        commentsSlot={
          <div className="lg:hidden">
            <CommentsList
              comments={comments}
              onAddComment={addComment}
              onUpdateComment={updateComment}
              onDeleteComment={deleteComment}
            />
          </div>
        }
      />

      <ConfirmDialog {...dialogProps} />
    </div>
  );
}

export default TicketDetail;
