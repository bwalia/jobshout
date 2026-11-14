import { afterEach, describe, expect, it, vi } from "vitest";
import { isDevLoginEnabled, signInAllowed } from "./auth";

const KEYS = ["AUTH_DEV_LOGIN", "APP_ENV", "NODE_ENV"] as const;

afterEach(() => {
  for (const k of KEYS) {
    vi.unstubAllEnvs();
  }
});

describe("isDevLoginEnabled", () => {
  it("is off by default", () => {
    vi.stubEnv("AUTH_DEV_LOGIN", "");
    vi.stubEnv("APP_ENV", "local");
    expect(isDevLoginEnabled()).toBe(false);
  });

  it("enables for local when AUTH_DEV_LOGIN=true", () => {
    vi.stubEnv("AUTH_DEV_LOGIN", "true");
    vi.stubEnv("APP_ENV", "local");
    expect(isDevLoginEnabled()).toBe(true);
  });

  it("never enables on int even with the flag", () => {
    vi.stubEnv("AUTH_DEV_LOGIN", "true");
    vi.stubEnv("APP_ENV", "int");
    expect(isDevLoginEnabled()).toBe(false);
  });

  it("never enables on prod", () => {
    vi.stubEnv("AUTH_DEV_LOGIN", "1");
    vi.stubEnv("APP_ENV", "prod");
    expect(isDevLoginEnabled()).toBe(false);
  });

  it("allows empty APP_ENV only outside NODE_ENV=production", () => {
    vi.stubEnv("AUTH_DEV_LOGIN", "true");
    vi.stubEnv("APP_ENV", "");
    vi.stubEnv("NODE_ENV", "development");
    expect(isDevLoginEnabled()).toBe(true);

    vi.stubEnv("NODE_ENV", "production");
    expect(isDevLoginEnabled()).toBe(false);
  });
});

describe("signInAllowed", () => {
  it("accepts a verified email (boolean or string claim)", () => {
    expect(signInAllowed({ email: "a@b.com", email_verified: true }, "a@b.com")).toBe(true);
    expect(signInAllowed({ email: "a@b.com", email_verified: "true" }, null)).toBe(true);
  });

  it("rejects an email the provider says is unverified", () => {
    expect(signInAllowed({ email: "staff@jobshout.com", email_verified: false }, "staff@jobshout.com")).toBe(false);
    expect(signInAllowed({ email: "a@b.com", email_verified: "false" }, "a@b.com")).toBe(false);
  });

  it("rejects a sign-in with no email at all", () => {
    expect(signInAllowed({}, null)).toBe(false);
    expect(signInAllowed(undefined, "")).toBe(false);
  });

  it("accepts providers without an email_verified claim (Facebook)", () => {
    expect(signInAllowed({ email: "a@b.com" }, "a@b.com")).toBe(true);
  });
});
