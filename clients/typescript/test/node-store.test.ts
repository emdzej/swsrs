import { mkdtempSync, rmSync, statSync } from "node:fs";
import { tmpdir, platform } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { FileTokenStore } from "../src/node.js";

describe("FileTokenStore", () => {
  let dir: string;
  let path: string;

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "swsrs-test-"));
    path = join(dir, "credentials.json");
  });

  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("returns null when the file doesn't exist", async () => {
    const s = new FileTokenStore(path);
    expect(await s.load()).toBeNull();
  });

  it("round-trips a token", async () => {
    const s = new FileTokenStore(path);
    const tok = { access_token: "atk", token_type: "Bearer", refresh_token: "rtk", expires_at: 12345 };
    await s.save(tok);
    expect(await s.load()).toEqual(tok);
  });

  it("writes with mode 0600 on Unix", async () => {
    if (platform() === "win32") return;
    const s = new FileTokenStore(path);
    await s.save({ access_token: "atk", token_type: "Bearer" });
    const mode = statSync(path).mode & 0o777;
    expect(mode).toBe(0o600);
  });

  it("clear() removes the file and is idempotent", async () => {
    const s = new FileTokenStore(path);
    await s.save({ access_token: "atk", token_type: "Bearer" });
    await s.clear();
    expect(await s.load()).toBeNull();
    // calling clear again should not throw
    await s.clear();
  });

  it("creates the parent directory if missing", async () => {
    const nested = join(dir, "a", "b", "creds.json");
    const s = new FileTokenStore(nested);
    await s.save({ access_token: "atk", token_type: "Bearer" });
    expect((await s.load())?.access_token).toBe("atk");
  });
});
