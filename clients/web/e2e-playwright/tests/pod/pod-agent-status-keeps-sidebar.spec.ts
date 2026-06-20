// Regression: a pod:agent_status_changed event carries an empty pod status.
// The Rust dispatch (event_dispatch.rs PodAgentStatusChanged) once funneled it
// through update_pod_status, blanking pod.status → the running pod dropped out
// of the "running,initializing" sidebar filter and vanished from the workspace
// mid-run. This locks the full wire → wasm dispatch → sidebar render chain.
import { test, expect } from "../../fixtures/index";
import { TEST_ORG_SLUG } from "../../helpers/env";
import { clearAuthRateLimit } from "../../helpers/redis";
import { terminateAllPods } from "../../helpers/pod-cleanup";
import { pollUntil } from "../../helpers/retry";
import { setupAcpScenarioPage } from "../../helpers/acp-spec-setup";
import { withEventSubscription } from "../../helpers/eventbus-stream";

test.describe("Pod sidebar · agent_status_changed keeps the running pod", () => {
  test.beforeEach(async () => { clearAuthRateLimit(); });
  test.afterEach(async () => { await terminateAllPods(); });

  test("an empty-status agent event must not drop the pod from the sidebar", async ({ page, api, monitor }) => {
    const cc = await api.connect();
    const token = api.getToken();
    if (!token) throw new Error("api fixture missing token");

    // Warm the workspace route — the first hit on a cold next_dev dev server
    // compiles the route + 21MB wasm, exceeding setupAcpScenarioPage's 30s goto.
    await page.goto(`/${TEST_ORG_SLUG}/workspace`, { waitUntil: "load", timeout: 120_000 });

    const { pod, assertWasmHealthy } = await setupAcpScenarioPage(page, api, monitor, {
      mode: "acp", scenario: "echo",
    });

    const podSelector = `[data-testid="pod-list-item"][data-pod-key="${pod.podKey}"]`;
    await expect(page.locator(podSelector)).toBeVisible({ timeout: 15_000 });

    // SendPodPrompt only drives the ACP executing→idle cycle once the agent
    // handshake settles to running; gate on it so the trigger can't race init.
    await pollUntil(
      async () => ((await cc.pod.getPod({ orgSlug: TEST_ORG_SLUG, podKey: pod.podKey })) as { status: string }).status === "running",
      { maxAttempts: 20, intervalMs: 1000, label: "pod-running" },
    );
    await page.waitForTimeout(1500); // wasm EventSubscriptionManager bootstrap

    const { event } = await withEventSubscription<unknown, { pod_key?: string; status?: string; agent_status?: string }>(
      {
        token, orgSlug: TEST_ORG_SLUG, timeoutMs: 15_000,
        predicate: (type, data) => type === "pod:agent_status_changed" && data.pod_key === pod.podKey,
      },
      async () => {
        await cc.pod.sendPodPrompt({ orgSlug: TEST_ORG_SLUG, podKey: pod.podKey, prompt: "ping" });
      },
    );
    expect(event.data.status ?? "").toBe("");
    expect(typeof event.data.agent_status).toBe("string");

    // The echoed reply proves the page consumed this pod's agent activity, so
    // its wasm subscriber has dispatched the empty-status event by now.
    await expect(page.getByText("echo: ping")).toBeVisible({ timeout: 15_000 });

    await expect(page.locator(podSelector)).toHaveCount(1);
    assertWasmHealthy();
  });
});
