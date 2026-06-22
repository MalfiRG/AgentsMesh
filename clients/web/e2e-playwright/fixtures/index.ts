import { test as base } from "@playwright/test";
import { DbFixture } from "./db.fixture";
import { ApiFixture } from "./api.fixture";
import { createConsoleMonitor, type ConsoleMonitor } from "../helpers/console-monitor";
import { seedRepoWithFakeProvider, type SeedRepoOptions, type SeededRepo } from "./seed-repo-with-fake-provider";

/**
 * Extended test fixtures with database and API helpers.
 *
 * Every spec that imports `{ test, expect }` from this module also
 * automatically gets a `monitor` fixture that records console.error +
 * pageerror entries from the page and asserts cleanness at teardown.
 * The default behavior is **deny** — any console.error/pageerror fails
 * the spec unless explicitly allowed via `monitor.allow(/regex/)`.
 *
 * The fixture is wired with `auto: true` and depends on `page`, so it
 * activates the moment a spec touches the page even if it never asks
 * for `monitor` by name. This makes it impossible to forget the
 * console assertion — exactly the failure mode that let the R6 WASM
 * regression slip through prior e2e coverage.
 *
 *     test("foo", async ({ page, monitor }) => {
 *       monitor.allow(/Failed to fetch deployment info/); // expected on dev
 *       await page.goto("/...");
 *     });
 */

interface Fixtures {
  db: DbFixture;
  api: ApiFixture;
  monitor: ConsoleMonitor;
  seedRepo: (opts?: SeedRepoOptions) => Promise<SeededRepo>;
}

export const test = base.extend<Fixtures>({
  db: async ({}, use) => {
    const db = new DbFixture();
    await use(db);
  },

  api: async ({}, use) => {
    const api = new ApiFixture();
    await use(api);
  },

  monitor: [
    async ({ page }, use) => {
      const monitor = createConsoleMonitor(page);
      try {
        await use(monitor);
        monitor.assertClean();
      } finally {
        monitor.dispose();
      }
    },
    { auto: true },
  ],

  seedRepo: async ({ db }, use) => {
    const seeded: SeededRepo[] = [];
    await use(async (opts?: SeedRepoOptions) => {
      const repo = await seedRepoWithFakeProvider(db, opts);
      seeded.push(repo);
      return repo;
    });
    for (const repo of seeded) {
      repo.cleanup();
    }
  },
});

export { expect } from "@playwright/test";
