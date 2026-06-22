import { test, expect } from "../../fixtures/index";
import { TEST_ORG_SLUG } from "../../helpers/env";
import { clearAuthRateLimit } from "../../helpers/redis";
import { terminateAllPods } from "../../helpers/pod-cleanup";
import { createMockAgentPod } from "../../helpers/mock-agent";
import { CreatePodModal } from "../../pages/modals/create-pod.modal";

test.describe("Ticket · pod refresh in detail view", () => {
  test.beforeEach(async () => { clearAuthRateLimit(); });
  test.afterEach(async () => { await terminateAllPods(); });

  test("same-tab: spawning a pod from the ticket sidebar refreshes the Working Pods rail without reload", async ({ page, api, monitor }) => {
    monitor.allow(/Failed to fetch deployment info/);

    const cc = await api.connect();
    const stamp = Date.now().toString(36);
    const ticket = (await cc.ticket.createTicket({
      orgSlug: TEST_ORG_SLUG,
      title: `e2e-pod-refresh-${stamp}`,
    })) as { slug: string };

    await page.goto(`/${TEST_ORG_SLUG}/tickets/${ticket.slug}`);
    await page.waitForLoadState("load");

    await expect(
      page.getByText(`e2e-pod-refresh-${stamp}`).first()
    ).toBeVisible({ timeout: 15_000 });

    await expect(page.getByText("Working Pods").first()).toBeVisible({ timeout: 10_000 });

    await page.getByRole("button", { name: /spawn pod/i }).first().click();

    const modal = new CreatePodModal(page);
    await modal.waitForOpen();

    const { builtinAgents: agents } = (await cc.agent.listAgents({ orgSlug: TEST_ORG_SLUG })) as {
      builtinAgents: Array<{ slug: string }>;
    };
    expect(agents.length, "dev env must have a builtin agent").toBeGreaterThan(0);
    await modal.selectAgent(agents[0].slug);
    await modal.submit();

    await modal.waitForClosed(20_000);

    await expect
      .poll(
        async () => {
          const { items } = (await cc.pod.listPods({ orgSlug: TEST_ORG_SLUG })) as {
            items?: Array<{ pod_key?: string; ticket?: { slug?: string }; status?: string }>;
          };
          return items?.filter(
            (p) => p.ticket?.slug === ticket.slug &&
              (p.status === "running" || p.status === "initializing"),
          ).length ?? 0;
        },
        { timeout: 20_000 },
      )
      .toBeGreaterThan(0);

    const workingPodsSection = page.getByTestId("working-pods-rail");
    await expect(workingPodsSection.getByRole("listitem").first()).toBeVisible({ timeout: 15_000 });

    await cc.ticket.deleteTicket({ orgSlug: TEST_ORG_SLUG, ticketSlug: ticket.slug }).catch(() => undefined);
  });

  test("realtime: pod:created with ticket_slug updates the Working Pods rail in a second tab", async ({ context, api }) => {
    const cc = await api.connect();
    const stamp = Date.now().toString(36);
    const ticket = (await cc.ticket.createTicket({
      orgSlug: TEST_ORG_SLUG,
      title: `e2e-rt-pod-refresh-${stamp}`,
    })) as { slug: string };

    const tabA = await context.newPage();
    const tabB = await context.newPage();

    await Promise.all([
      tabA.goto(`/${TEST_ORG_SLUG}/tickets/${ticket.slug}`),
      tabB.goto(`/${TEST_ORG_SLUG}/tickets/${ticket.slug}`),
    ]);
    await Promise.all([
      tabA.waitForLoadState("load"),
      tabB.waitForLoadState("load"),
    ]);

    await Promise.all([
      expect(tabA.getByText(`e2e-rt-pod-refresh-${stamp}`).first()).toBeVisible({ timeout: 15_000 }),
      expect(tabB.getByText(`e2e-rt-pod-refresh-${stamp}`).first()).toBeVisible({ timeout: 15_000 }),
    ]);

    // "No active pods" renders once useTicketPods resolves with an empty list,
    // proving tab B's sidebar is hydrated and the wasm subscription is active.
    await Promise.all([
      expect(tabA.getByTestId("working-pods-rail").getByText("No active pods")).toBeVisible({ timeout: 10_000 }),
      expect(tabB.getByTestId("working-pods-rail").getByText("No active pods")).toBeVisible({ timeout: 10_000 }),
    ]);

    await tabA.getByRole("button", { name: /spawn pod/i }).first().click();
    const modal = new CreatePodModal(tabA);
    await modal.waitForOpen();
    const { builtinAgents: agents } = (await cc.agent.listAgents({ orgSlug: TEST_ORG_SLUG })) as {
      builtinAgents: Array<{ slug: string }>;
    };
    expect(agents.length, "dev env must have a builtin agent").toBeGreaterThan(0);
    await modal.selectAgent(agents[0].slug);
    await modal.submit();
    await modal.waitForClosed(20_000);

    await expect
      .poll(
        async () => {
          const { items } = (await cc.pod.listPods({ orgSlug: TEST_ORG_SLUG })) as {
            items?: Array<{ pod_key?: string; ticket?: { slug?: string }; status?: string }>;
          };
          return items?.filter(
            (p) => p.ticket?.slug === ticket.slug &&
              (p.status === "running" || p.status === "initializing"),
          ).length ?? 0;
        },
        { timeout: 20_000 },
      )
      .toBeGreaterThan(0);

    await expect(
      tabB.getByTestId("working-pods-rail").getByRole("listitem").first()
    ).toBeVisible({ timeout: 20_000 });

    await tabA.close();
    await tabB.close();
    await cc.ticket.deleteTicket({ orgSlug: TEST_ORG_SLUG, ticketSlug: ticket.slug }).catch(() => undefined);
  });

  test("realtime-api: pod:created wire event carries ticket_slug when pod created via API with ticket context", async ({ api }) => {
    const { withEventSubscription } = await import("../../helpers/eventbus-stream");

    const cc = await api.connect();
    const token = api.getToken();
    if (!token) throw new Error("api fixture missing token");

    const stamp = Date.now().toString(36);
    const ticket = (await cc.ticket.createTicket({
      orgSlug: TEST_ORG_SLUG,
      title: `e2e-wire-ticket-slug-${stamp}`,
    })) as { slug: string };

    let createdPodKey: string | undefined;

    const { event } = await withEventSubscription<
      unknown,
      { pod_key?: string; ticket_slug?: string }
    >(
      {
        token,
        orgSlug: TEST_ORG_SLUG,
        predicate: (type, data) =>
          type === "pod:created" &&
          typeof data.pod_key === "string" &&
          data.pod_key === createdPodKey,
        timeoutMs: 15_000,
      },
      async () => {
        const pod = await createMockAgentPod(api, {
          mode: "pty",
          scenario: "echo",
          alias: `ticket-${ticket.slug}`,
          ticketSlug: ticket.slug,
        });
        createdPodKey = pod.podKey;
      },
    );

    expect(event.type).toBe("pod:created");
    expect(event.data.pod_key).toBe(createdPodKey);
    expect(event.data.ticket_slug).toBe(ticket.slug);

    await cc.ticket.deleteTicket({ orgSlug: TEST_ORG_SLUG, ticketSlug: ticket.slug }).catch(() => undefined);
  });
});
