import { describe, expect, it, vi } from "vitest";
import { AdminClient, AdminError } from "../src/admin.js";

function mockFetch(handler: (req: Request) => Response | Promise<Response>): typeof fetch {
  return async (input, init) => {
    const req = input instanceof Request ? input : new Request(input as string, init);
    return handler(req);
  };
}

describe("AdminClient", () => {
  it("createSession posts to /admin/sessions with bearer", async () => {
    const fetch = mockFetch(async (req) => {
      expect(req.method).toBe("POST");
      expect(req.url).toBe("https://relay.example.com/admin/sessions");
      expect(req.headers.get("Authorization")).toBe("Bearer test-token");
      return new Response(
        JSON.stringify({
          id: "s1",
          initiator_token: "i",
          responder_token: "r",
          created_at: "now",
          expires_at: "later",
        }),
        { status: 201 },
      );
    });
    const c = new AdminClient({ baseURL: "https://relay.example.com", token: () => "test-token", fetch });
    const s = await c.createSession();
    expect(s.id).toBe("s1");
  });

  it("calls the token provider per request (supports rotation)", async () => {
    let n = 0;
    const tokens = ["t1", "t2"];
    const seen: string[] = [];
    const fetch = mockFetch((req) => {
      seen.push(req.headers.get("Authorization") ?? "");
      return new Response(JSON.stringify({ sessions: [] }), { status: 200 });
    });
    const c = new AdminClient({
      baseURL: "https://relay.example.com",
      token: async () => tokens[n++ % tokens.length]!,
      fetch,
    });
    await c.listSessions();
    await c.listSessions();
    expect(seen).toEqual(["Bearer t1", "Bearer t2"]);
  });

  it("throws AdminError on non-2xx with the status code", async () => {
    const fetch = mockFetch(() => new Response("nope", { status: 403, statusText: "Forbidden" }));
    const c = new AdminClient({ baseURL: "https://relay.example.com", token: () => "x", fetch });
    await expect(c.createSession()).rejects.toBeInstanceOf(AdminError);
    try {
      await c.createSession();
    } catch (e) {
      expect((e as AdminError).status).toBe(403);
    }
  });

  it("deleteSession sends DELETE and tolerates 204", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 })) as unknown as typeof fetch;
    const c = new AdminClient({ baseURL: "https://relay.example.com", token: () => "x", fetch });
    await c.deleteSession("abc");
    const call = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0]!;
    expect(call[0]).toBe("https://relay.example.com/admin/sessions/abc");
    expect(call[1]?.method).toBe("DELETE");
  });

  it("URL-encodes the session id in path operations", async () => {
    let seenURL = "";
    const fetch = mockFetch(async (req) => {
      seenURL = req.url;
      return new Response(JSON.stringify({}), { status: 200 });
    });
    const c = new AdminClient({ baseURL: "https://relay.example.com", token: () => "x", fetch });
    await c.getSession("has/slash and space");
    expect(seenURL).toBe("https://relay.example.com/admin/sessions/has%2Fslash%20and%20space");
  });
});
