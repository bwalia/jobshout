import { test, expect } from "@playwright/test";
import { uniqueEmail } from "./helpers";

const signupUser = {
  fullName: "Signup Tester",
  email: uniqueEmail("auth"),
  orgName: `Auth Test Org ${Date.now()}`,
  password: "testpass1234",
};

test.describe("Authentication", () => {
  test("signup creates account and redirects to dashboard", async ({
    page,
  }) => {
    await page.goto("/signup");

    await page.fill("#fullName", signupUser.fullName);
    await page.fill("#email", signupUser.email);
    await page.fill("#orgName", signupUser.orgName);
    await page.fill("#password", signupUser.password);
    await page.click('button[type="submit"]');

    await page.waitForURL("**/dashboard", { timeout: 15_000 });
    await expect(page.locator("h1")).toContainText("Dashboard");
  });

  test("login with registered user redirects to dashboard", async ({
    page,
  }) => {
    await page.goto("/login");

    await page.fill("#email", signupUser.email);
    await page.fill("#password", signupUser.password);
    await page.click('button[type="submit"]');

    await page.waitForURL("**/dashboard", { timeout: 15_000 });
    await expect(page.locator("h1")).toContainText("Dashboard");
  });

  test("login with wrong password shows error", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    await page.goto("http://localhost:3001/login");
    await page.waitForSelector('button:has-text("Sign in")', {
      timeout: 5_000,
    });

    // Use type() to simulate real keystrokes for controlled React inputs
    await page.locator("#email").click();
    await page.locator("#email").type(signupUser.email);
    await page.locator("#password").click();
    await page.locator("#password").type("wrongpassword");
    await page.click('button:has-text("Sign in")');

    // Should stay on login page and not redirect to dashboard
    await page.waitForTimeout(3_000);
    await expect(page).toHaveURL(/login/);
    await context.close();
  });

  test("signup with duplicate email shows error", async ({ browser }) => {
    const context = await browser.newContext();
    const page = await context.newPage();

    await page.goto("http://localhost:3001/signup");
    await page.waitForSelector('button:has-text("Create account")', {
      timeout: 5_000,
    });

    await page.locator("#fullName").click();
    await page.locator("#fullName").type("Duplicate");
    await page.locator("#email").click();
    await page.locator("#email").type(signupUser.email);
    await page.locator("#orgName").click();
    await page.locator("#orgName").type(`Dup Org ${Date.now()}`);
    await page.locator("#password").click();
    await page.locator("#password").type("testpass1234");
    await page.click('button:has-text("Create account")');

    // Should stay on signup page and not redirect to dashboard
    await page.waitForTimeout(3_000);
    await expect(page).toHaveURL(/signup/);
    await context.close();
  });

  test("unauthenticated user is redirected to login", async ({ page }) => {
    // Use a fresh context without any stored auth
    const context = await page.context().browser()!.newContext();
    const freshPage = await context.newPage();
    await freshPage.goto("http://localhost:3001/dashboard");
    await freshPage.waitForURL("**/login", { timeout: 10_000 });
    await context.close();
  });
});
