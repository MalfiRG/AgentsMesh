import type { Locator, Page } from "@playwright/test";
import { TEST_ORG_SLUG } from "../helpers/env";

export class CreatePodPage {
  private readonly dialog: Locator;

  constructor(private readonly page: Page) {
    this.dialog = page.locator('[role="dialog"]').first();
  }

  async gotoWorkspaceCreate(): Promise<void> {
    await this.page.goto(`/${TEST_ORG_SLUG}/workspace`);
    await this.page.waitForLoadState("load");

    const btn = this.page
      .getByRole("button", { name: /new pod|create new pod|新建 pod/i })
      .first();
    await btn.click();
    await this.dialog.waitFor({ state: "visible" });
  }

  async openAdvanced(): Promise<void> {
    const trigger = this.dialog.getByRole("button", {
      name: /advanced options|高级选项/i,
    });
    await trigger.waitFor({ state: "visible" });
    const state = await trigger.getAttribute("data-state");
    if (state !== "open") {
      await trigger.click();
    }
  }

  async selectRepository(slug?: string): Promise<void> {
    const select = this.dialog.locator("select#repository-select");
    await select.waitFor({ state: "visible" });
    if (slug) {
      await select.selectOption({ label: slug });
    } else {
      const options = await select.locator("option").all();
      for (const opt of options) {
        const val = await opt.getAttribute("value");
        if (val) {
          await select.selectOption(val);
          return;
        }
      }
    }
  }

  branchCombobox(): Locator {
    return this.dialog.getByLabel(/branch/i, { exact: false });
  }

  branchOption(name: string): Locator {
    return this.dialog.getByRole("option", { name });
  }

  async submit(): Promise<void> {
    await this.dialog
      .getByRole("button", { name: /create|创建/i })
      .click();
  }
}
