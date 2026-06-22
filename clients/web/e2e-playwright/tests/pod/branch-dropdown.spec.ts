import { test, expect } from "../../fixtures/index";
import { seedRepoWithFakeProvider } from "../../fixtures/seed-repo-with-fake-provider";
import { CreatePodPage } from "../../pages/create-pod.page";

test.describe("Branch dropdown in CreatePod", () => {
  test("branches load lazily on dropdown open, not on form mount", async ({ page, db, monitor }) => {
    monitor.allow(/Failed to fetch deployment info/);
    monitor.allow(/listRepositoryBranches/);

    const repo = await seedRepoWithFakeProvider(db, { branches: ["main", "develop"] });
    try {
      const createPod = new CreatePodPage(page);
      await createPod.gotoWorkspaceCreate();
      await createPod.openAdvanced();
      await createPod.selectRepository(repo.slug);

      await expect(createPod.branchOption("develop")).toHaveCount(0);

      await createPod.branchCombobox().focus();

      await expect(createPod.branchOption("develop")).toBeVisible();
      await expect(createPod.branchOption("main")).toBeVisible();
    } finally {
      repo.cleanup();
    }
  });

  test("branch fetch failure falls back to free-text input", async ({ page, db, monitor }) => {
    monitor.allow(/Failed to fetch deployment info/);
    monitor.allow(/listRepositoryBranches/);
    monitor.allow(/Failed to load resource:.*status of 5[0-9]{2}/i);

    const repo = await seedRepoWithFakeProvider(db, { status: 500 });
    try {
      const createPod = new CreatePodPage(page);
      await createPod.gotoWorkspaceCreate();
      await createPod.openAdvanced();
      await createPod.selectRepository(repo.slug);

      await createPod.branchCombobox().focus();

      await expect(createPod.branchCombobox()).not.toBeVisible();

      const freeText = page.locator('[role="dialog"] input#branch-input');
      await freeText.waitFor({ state: "visible" });
      await freeText.fill("feature/typed");
      await expect(freeText).toHaveValue("feature/typed");
    } finally {
      repo.cleanup();
    }
  });

  test("select a branch then create a pod", async ({ page, db, api, monitor }) => {
    monitor.allow(/Failed to fetch deployment info/);
    monitor.allow(/listRepositoryBranches/);

    const repo = await seedRepoWithFakeProvider(db, { branches: ["main", "develop"] });
    try {
      const cc = await api.connect();
      const { builtinAgents: agents } = await cc.agent.listAgents({
        orgSlug: "dev-org",
      }) as { builtinAgents: Array<{ slug: string }> };
      expect(agents.length, "dev env must have a builtin agent").toBeGreaterThan(0);

      const podsBefore = await cc.pod.listPods({ orgSlug: "dev-org" }) as { total: bigint | number };
      const beforeTotal = Number(podsBefore.total);

      const createPod = new CreatePodPage(page);
      await createPod.gotoWorkspaceCreate();
      await createPod.openAdvanced();
      await createPod.selectRepository(repo.slug);

      await createPod.branchCombobox().focus();
      await expect(createPod.branchOption("develop")).toBeVisible();

      await page.getByRole("option", { name: "develop" }).click({ force: true });
      await expect(createPod.branchCombobox()).toHaveValue("develop");

      await createPod.submit();

      await expect
        .poll(
          async () => {
            const after = await cc.pod.listPods({ orgSlug: "dev-org" }) as { total: bigint | number };
            return Number(after.total);
          },
          { timeout: 15_000 },
        )
        .toBeGreaterThan(beforeTotal);
    } finally {
      repo.cleanup();
    }
  });
});
