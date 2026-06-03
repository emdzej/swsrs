import { describe, expect, it } from "vitest";
import { MemoryTokenStore } from "../src/store.js";

describe("MemoryTokenStore", () => {
  it("round-trips tokens and clears", async () => {
    const s = new MemoryTokenStore();
    expect(await s.load()).toBeNull();

    await s.save({ access_token: "atk", token_type: "Bearer" });
    const loaded = await s.load();
    expect(loaded?.access_token).toBe("atk");

    await s.clear();
    expect(await s.load()).toBeNull();
  });
});
