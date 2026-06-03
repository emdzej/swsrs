#!/usr/bin/env node
//
// swsrs-chat: a tiny two-party text chat over a swsrs relay session.
//
//   # A side: create a session and start chatting (also prints the token
//   #         the B side needs)
//   swsrs-chat host --relay wss://relay.example.com [--admin URL] [--token OIDC]
//
//   # B side: join an existing session
//   swsrs-chat join --relay wss://relay.example.com --session <id> --token <responder-token>
//
// Each side reads lines from stdin and sends them to the peer; received
// messages are printed to stdout, tagged with the sender.

import { Command, Option } from "commander";
import { createInterface } from "node:readline";
import { stdin, stdout } from "node:process";
import { AdminClient, dial, accept } from "@emdzej/swsrs-client";

interface CommonOpts {
  relay: string;
  name: string;
}

interface HostOpts extends CommonOpts {
  admin?: string;
  token?: string;
}

interface JoinOpts extends CommonOpts {
  session: string;
  token: string;
}

const program = new Command();
program
  .name("swsrs-chat")
  .description("Two-party text chat over a swsrs relay session")
  .version("0.0.0");

program
  .command("host")
  .description("Create a session, print the responder token, and chat as initiator")
  .requiredOption("--relay <url>", "relay base URL, e.g. wss://relay.example.com")
  .option("--admin <url>", "admin base URL (defaults to relay URL)")
  .option("--token <oidc>", "OIDC bearer token for the admin API (omit when relay runs --no-auth)")
  .addOption(new Option("--name <name>", "display name for outgoing messages").default("host"))
  .action(async (opts: HostOpts) => {
    await runHost(opts);
  });

program
  .command("join")
  .description("Connect to an existing session as responder and chat")
  .requiredOption("--relay <url>", "relay base URL")
  .requiredOption("--session <id>", "session id from the host")
  .requiredOption("--token <token>", "responder token from the host")
  .addOption(new Option("--name <name>", "display name for outgoing messages").default("guest"))
  .action(async (opts: JoinOpts) => {
    await runJoin(opts);
  });

await program.parseAsync(process.argv);

// --------------------------------------------------------------------------

async function runHost(opts: HostOpts): Promise<void> {
  const adminURL = (opts.admin ?? toHttp(opts.relay)).replace(/\/$/, "");
  const admin = new AdminClient({
    baseURL: adminURL,
    token: () => opts.token ?? "",
  });

  console.error(`[swsrs-chat] creating session via ${adminURL}...`);
  const session = await admin.createSession();
  console.error(`[swsrs-chat] session id:        ${session.id}`);
  console.error(`[swsrs-chat] responder token:   ${session.responder_token}`);
  console.error(`[swsrs-chat] expires:           ${session.expires_at}`);
  console.error(`[swsrs-chat] tell the other side to run:`);
  console.error(
    `  swsrs-chat join --relay ${opts.relay} --session ${session.id} --token ${session.responder_token}`,
  );
  console.error(`[swsrs-chat] waiting for peer to attach...`);

  const conn = await dial({
    relayURL: opts.relay,
    sessionId: session.id,
    token: session.initiator_token,
  });
  console.error(`[swsrs-chat] connected. type messages, ctrl-d to quit.`);
  runChat(conn, opts.name);
}

async function runJoin(opts: JoinOpts): Promise<void> {
  console.error(`[swsrs-chat] joining session ${opts.session}...`);
  const conn = await accept({
    relayURL: opts.relay,
    sessionId: opts.session,
    token: opts.token,
  });
  console.error(`[swsrs-chat] connected. type messages, ctrl-d to quit.`);
  runChat(conn, opts.name);
}

// runChat wires the connection up to stdin/stdout. Each line of stdin is sent
// as one binary frame; each received frame is decoded as UTF-8 and printed.
function runChat(
  conn: Awaited<ReturnType<typeof dial>>,
  selfName: string,
): void {
  const encoder = new TextEncoder();
  const decoder = new TextDecoder();

  conn.socket.addEventListener("message", (e: MessageEvent) => {
    const data = e.data;
    let text: string;
    if (data instanceof ArrayBuffer) {
      text = decoder.decode(new Uint8Array(data));
    } else if (typeof data === "string") {
      text = data;
    } else {
      text = "(binary)";
    }
    // Carriage-return then write so the prompt doesn't get clobbered by
    // an in-progress input line. Imperfect but adequate for a demo.
    stdout.write(`\r${text}\n`);
  });

  conn.closed.then((e: CloseEvent) => {
    console.error(`[swsrs-chat] disconnected (${e.code} ${e.reason || "no reason"})`);
    process.exit(0);
  });

  const rl = createInterface({ input: stdin, terminal: false });
  rl.on("line", (line: string) => {
    if (line.length === 0) return;
    conn.send(encoder.encode(`${selfName}: ${line}`));
  });
  rl.on("close", () => {
    conn.close(1000, "stdin closed");
  });
}

function toHttp(u: string): string {
  return u.replace(/^wss:\/\//, "https://").replace(/^ws:\/\//, "http://");
}
