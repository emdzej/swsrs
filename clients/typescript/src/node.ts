// @emdzej/swsrs-client/node — Node-only utilities. Browsers can't import
// these (they reference node:fs / node:os).

import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { homedir, platform } from "node:os";
import { dirname, join } from "node:path";
import type { TokenResponse } from "./auth.js";
import type { TokenStore } from "./store.js";

/**
 * Returns the OS-appropriate default credentials path:
 *   - Linux/BSD:  $XDG_CONFIG_HOME/swsrs/credentials.json
 *                 (or ~/.config/swsrs/credentials.json)
 *   - macOS:      ~/Library/Application Support/swsrs/credentials.json
 *   - Windows:    %APPDATA%\swsrs\credentials.json
 */
export function defaultCredentialsPath(): string {
  const env = process.env;
  if (platform() === "win32") {
    const appData = env["APPDATA"] ?? join(homedir(), "AppData", "Roaming");
    return join(appData, "swsrs", "credentials.json");
  }
  if (platform() === "darwin") {
    return join(homedir(), "Library", "Application Support", "swsrs", "credentials.json");
  }
  const xdg = env["XDG_CONFIG_HOME"] ?? join(homedir(), ".config");
  return join(xdg, "swsrs", "credentials.json");
}

/**
 * Persists tokens to a JSON file with mode 0600. Atomic — writes to a temp
 * file then renames.
 *
 * The file format is shared with the Go SDK and `swsrs auth`, which use the
 * same default path: `expiry` (RFC 3339) is written alongside `expires_at`,
 * and `expires_at` is derived from `expiry` when reading a Go-written file.
 */
export class FileTokenStore implements TokenStore {
  constructor(private readonly path: string = defaultCredentialsPath()) {}

  async load(): Promise<TokenResponse | null> {
    try {
      const data = await readFile(this.path, "utf8");
      const { expiry, ...tok } = JSON.parse(data) as TokenResponse & { expiry?: string };
      if (tok.expires_at === undefined && expiry) {
        const t = Date.parse(expiry);
        if (!Number.isNaN(t)) tok.expires_at = t;
      }
      return tok;
    } catch (e: unknown) {
      if ((e as NodeJS.ErrnoException).code === "ENOENT") return null;
      throw e;
    }
  }

  async save(token: TokenResponse): Promise<void> {
    await mkdir(dirname(this.path), { recursive: true, mode: 0o700 });
    const tmp = `${this.path}.${process.pid}.tmp`;
    const onDisk =
      token.expires_at !== undefined ? { ...token, expiry: new Date(token.expires_at).toISOString() } : token;
    await writeFile(tmp, JSON.stringify(onDisk, null, 2), { mode: 0o600 });
    await rename(tmp, this.path);
  }

  async clear(): Promise<void> {
    await rm(this.path, { force: true });
  }
}
