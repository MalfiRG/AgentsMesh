export function TicketDetailSkeleton() {
  return (
    <div className="animate-pulse" data-testid="ticket-detail-skeleton">
      <div className="flex flex-col lg:flex-row gap-6 lg:gap-8">
        <div className="flex-1 space-y-6">
          <div className="space-y-4">
            <div className="flex items-center gap-2.5">
              <div className="h-5 w-20 bg-muted/60 rounded" />
              <div className="h-5 w-24 bg-muted/60 rounded-full" />
            </div>
            <div className="h-8 bg-muted/60 rounded-lg w-3/4" />
          </div>
          <div className="h-10 bg-muted/40 rounded-lg w-full" />
          <div className="h-64 bg-muted/40 rounded-xl" />
        </div>
        <div className="lg:w-72 shrink-0 space-y-3">
          <div className="h-[52px] bg-muted/50 rounded-xl" />
          <div className="rounded-xl border border-border/40 overflow-hidden">
            <div className="h-12 bg-muted/30" />
            <div className="h-12 bg-muted/20" />
            <div className="h-12 bg-muted/30" />
            <div className="h-16 bg-muted/20" />
            <div className="h-10 bg-muted/30" />
          </div>
          <div className="h-9 bg-muted/30 rounded-lg" />
        </div>
      </div>
    </div>
  );
}

export default TicketDetailSkeleton;
