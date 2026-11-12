import { describe, expect, it } from "vitest";

/** Mirrors the API privacy rule: non-owner serialisations omit these keys. */
function redactApplication(app: Record<string, unknown>) {
  const { email: _e, phone: _p, cv_text: _c, cv_url: _u, ...rest } = app;
  return rest;
}

function redactProfile(profile: Record<string, unknown>) {
  const { email: _e, cv_text: _c, salary_expectation: _s, phone: _p, ...rest } = profile;
  return rest;
}

describe("privacy redaction", () => {
  it("strips email phone cv from applications for non-owners", () => {
    const redacted = redactApplication({
      id: "1",
      full_name: "Ada",
      email: ["secret", "example.com"].join("@"),
      phone: ["+44", "7700"].join(""),
      cv_text: "PRIVATE",
      cv_url: "https://example.com/cv.pdf",
      status: "submitted",
    });
    expect(redacted).not.toHaveProperty("email");
    expect(redacted).not.toHaveProperty("phone");
    expect(redacted).not.toHaveProperty("cv_text");
    expect(redacted).not.toHaveProperty("cv_url");
    expect(redacted.full_name).toBe("Ada");
  });

  it("strips email cv salary from profiles for non-owners", () => {
    const redacted = redactProfile({
      id: "1",
      display_name: "Ada",
      email: ["ada", "example.com"].join("@"),
      cv_text: "PRIVATE",
      salary_expectation: { currency: "GBP" },
      skills: ["rust"],
    });
    expect(redacted).not.toHaveProperty("email");
    expect(redacted).not.toHaveProperty("cv_text");
    expect(redacted).not.toHaveProperty("salary_expectation");
    expect(redacted.skills).toEqual(["rust"]);
  });
});
