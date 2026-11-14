import { describe, expect, it } from "vitest";
import {
  formatDeployIdentity,
  formatDeployedAtGmt,
} from "./deploy-identity";

describe("deploy-identity", () => {
  it("formats the full operator line", () => {
    expect(
      formatDeployIdentity({
        env: "int",
        version: "v1.0.1",
        gitSha: "c82fc12abcdef",
        buildId: "43474661245",
        deployedAt: "2026-11-13T17:38:00Z",
      })
    ).toBe("int · v1.0.1 · c82fc12 · Build #43474661245 · 13 Nov 2026, 17:38 GMT");
  });

  it("omits missing fields and latest-only version", () => {
    expect(formatDeployIdentity({ version: "v1.0.1", env: "prod" })).toBe(
      "prod · v1.0.1"
    );
    expect(formatDeployIdentity({ version: "latest", env: "int" })).toBe("int");
    expect(formatDeployIdentity({})).toBe("");
  });

  it("formats GMT timestamps", () => {
    expect(formatDeployedAtGmt("2026-11-13T17:38:00Z")).toBe(
      "13 Nov 2026, 17:38 GMT"
    );
    expect(formatDeployedAtGmt("not-a-date")).toBeUndefined();
  });
});
