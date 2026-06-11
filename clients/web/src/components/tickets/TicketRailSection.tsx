"use client";

interface RailSectionProps {
  title: string;
  count?: number;
  action?: React.ReactNode;
  children: React.ReactNode;
}

export function RailSection({ title, count, action, children }: RailSectionProps) {
  return (
    <section className="rounded-md border border-border bg-card">
      <header className="flex items-center justify-between border-b border-border px-3 py-2">
        <span className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
          {title}
        </span>
        <div className="flex items-center gap-1.5">
          {typeof count === "number" && count > 0 && (
            <span className="font-mono text-[11px] text-muted-foreground">{count}</span>
          )}
          {action}
        </div>
      </header>
      <div className="p-2">{children}</div>
    </section>
  );
}

export function RailEmpty({ icon, text }: { icon: React.ReactNode; text: string }) {
  return (
    <div className="flex items-center gap-2 px-2 py-3 text-[12px] text-muted-foreground/70">
      {icon}
      <span>{text}</span>
    </div>
  );
}
