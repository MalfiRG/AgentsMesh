import { toast } from "sonner";
import {
  type RealtimeEvent,
  decodeEventData,
  NotificationPayloadEventDataSchema,
} from "@/lib/realtime";

export function handleNotificationEvent(
  event: RealtimeEvent,
  opts: {
    router: { push: (url: string) => void };
    showBrowserNotification: (data: { title: string; body: string; link?: string }) => void;
  }
): void {
  if (event.type !== "notification") return;

  const data = decodeEventData(NotificationPayloadEventDataSchema, event.data);
  const wantsToast = !!data.channels?.toast;
  const wantsBrowser = !!data.channels?.browser;
  const tabVisible = typeof document !== "undefined" && document.visibilityState === "visible";
  const showToast = wantsToast && (!wantsBrowser || tabVisible);
  const showBrowser = wantsBrowser && (!wantsToast || !tabVisible);

  if (showToast) {
    const toastFn = data.priority === "high" ? toast.warning : toast.info;
    toastFn(data.title, {
      id: `notif:${data.source}:${data.title}:${data.body}:${data.link ?? ""}`,
      description: data.body,
      duration: data.priority === "high" ? 8000 : 4000,
      ...(data.link ? { action: { label: "→", onClick: () => opts.router.push(data.link) } } : {}),
    });
  }

  if (showBrowser) {
    opts.showBrowserNotification({ title: data.title, body: data.body, link: data.link });
  }
}
