import { mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
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

  it("writes expiry for the Go SDK", async () => {
    const s = new FileTokenStore(path);
    await s.save({ access_token: "atk", token_type: "Bearer", expires_at: Date.UTC(2030, 0, 1) });
    expect(JSON.parse(readFileSync(path, "utf8")).expiry).toBe("2030-01-01T00:00:00.000Z");
  });

  it("reads a file written by the Go SDK", async () => {
    writeFileSync(
      path,
      JSON.stringify({
        access_token: "atk",
        token_type: "Bearer",
        refresh_token: "rtk",
        expiry: "2030-01-01T00:00:00Z",
        client_id: "cli",
      }),
    );
    const tok = await new FileTokenStore(path).load();
    expect(tok).toEqual({
      access_token: "atk",
      token_type: "Bearer",
      refresh_token: "rtk",
      expires_at: Date.UTC(2030, 0, 1),
      client_id: "cli",
    });
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
