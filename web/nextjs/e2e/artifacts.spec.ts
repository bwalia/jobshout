import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo } from "./helpers";

let creds: { email: string; password: string; token: string };

test.describe("Artifacts", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("artifacts");
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  test("library shows an empty state and type tabs", async ({ page }) => {
    await navigateTo(page, "/panel/artifacts");
    await expect(page.locator("h1")).toContainText("Artifacts");
    await expect(page.getByRole("button", { name: "All" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Articles", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Images", exact: true })).toBeVisible();
    await expect(page.getByText("No artifacts yet.")).toBeVisible({
      timeout: 5_000,
    });

    await page.getByRole("button", { name: "Write articles" }).click();
    await expect(page.getByRole("heading", { name: "Write articles" })).toBeVisible();
    await page.getByRole("button", { name: "Cancel" }).click();

    await page.getByRole("button", { name: "Articles", exact: true }).click();
    await expect(page.getByText("No articles yet.")).toBeVisible();
    await page.getByRole("button", { name: "Images", exact: true }).click();
    await expect(page.getByText("No images yet.")).toBeVisible();
  });

  test("/artifacts redirects to the panel", async ({ page }) => {
    await page.goto("/artifacts");
    await page.waitForURL("**/panel/artifacts**", { timeout: 10_000 });
    await expect(page.locator("h1")).toContainText("Artifacts");
  });

  test("a started article run appears in the library", async ({ page }) => {
    const API_URL = process.env.E2E_API_URL ?? "http://localhost:8090/api/v1";
    const start = await fetch(`${API_URL}/blogs/generate`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${creds.token}`,
      },
      body: JSON.stringify({
        briefs: [{ topic: "Artifacts library listing test" }],
      }),
    });
    expect(start.status).toBe(202);

    await navigateTo(page, "/panel/artifacts");
    await expect(
      page.getByRole("heading", { name: "Artifacts library listing test" })
    ).toBeVisible({ timeout: 10_000 });
    await page.getByRole("button", { name: "Articles", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "Artifacts library listing test" })
    ).toBeVisible();
  });

  test("Images tab empty state does not offer write-article", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/artifacts?kind=image");
    await expect(page.getByText("No images yet.")).toBeVisible({
      timeout: 5_000,
    });
    await expect(
      page.getByRole("button", { name: "Write your first article" })
    ).toHaveCount(0);
  });
});
