import type { TokenResponse } from "./auth.js";

/** Pluggable token persistence. Returns null when no token has been stored. */
export interface TokenStore {
  load(): Promise<TokenResponse | null>;
  save(token: TokenResponse): Promise<void>;
  clear(): Promise<void>;
}

/** In-memory store. Useful for tests and ephemeral apps. */
export class MemoryTokenStore implements TokenStore {
  private tok: TokenResponse | null = null;
  async load(): Promise<TokenResponse | null> {
    return this.tok;
  }
  async save(token: TokenResponse): Promise<void> {
    this.tok = token;
  }
  async clear(): Promise<void> {
    this.tok = null;
  }
}
