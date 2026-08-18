import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import type { EtlTableSpec } from "./etl-instance.ts";

export class SrAccessError extends Error {
  readonly code = "SR_UNREACHABLE";
  constructor(
    readonly reason: "igate" | "auth" | "network" | "other",
    message: string,
  ) {
    super(message);
    this.name = "SrAccessError";
  }
}

export type SrConfig = {
  readonly host: string;
  readonly port: number;
  readonly user: string;
  readonly password: string;
};

export function loadDotEnv(file: string): void {
  if (!existsSync(file)) return;
  for (const line of readFileSync(file, "utf8").split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const eq = trimmed.indexOf("=");
    if (eq < 1) continue;
    const key = trimmed.slice(0, eq);
    const value = trimmed.slice(eq + 1);
    if (process.env[key] === undefined) process.env[key] = value;
  }
}

export function srConfigFromEnv(envDir = process.cwd()): SrConfig {
  loadDotEnv(path.join(envDir, ".env"));
  const host = process.env.HIPPO_SR_HOST;
  const user = process.env.HIPPO_SR_USER;
  const password = process.env.HIPPO_SR_PASSWORD ?? "";
  const port = Number(process.env.HIPPO_SR_PORT ?? "9030");
  if (!host || !user) {
    throw new SrAccessError("other", "HIPPO_SR_HOST / HIPPO_SR_USER missing in .env");
  }
  return { host, port, user, password };
}

function classifySrError(err: unknown): SrAccessError {
  const message = err instanceof Error ? err.message : String(err);
  if (
    message.includes("Packet sequence number wrong") ||
    message.includes("HTTP/1.1 403") ||
    message.includes("igate") ||
    message.includes("IDC")
  ) {
    return new SrAccessError(
      "igate",
      `DevCloud → IDC blocked by igate (${message}). Host is reachable on TCP but speaks HTTP 403, not MySQL.`,
    );
  }
  if (message.includes("Access denied") || message.includes("1045")) {
    return new SrAccessError("auth", message);
  }
  if (message.includes("ECONNREFUSED") || message.includes("ETIMEDOUT") || message.includes("ENOTFOUND")) {
    return new SrAccessError("network", message);
  }
  if (message.includes("Connection lost") || message.includes("server closed the connection")) {
    return new SrAccessError(
      "igate",
      `MySQL handshake dropped (${message}). From this DevCloud VM, omnicontext FE answers HTTP 403 (igate), not the StarRocks MySQL protocol.`,
    );
  }
  return new SrAccessError("other", message);
}

export type SrRow = Record<string, unknown>;

export type SrQuery = {
  describe(spec: EtlTableSpec): Promise<string[]>;
  query(spec: EtlTableSpec, limit?: number): Promise<SrRow[]>;
  close(): Promise<void>;
};

export async function openSr(config: SrConfig): Promise<SrQuery> {
  let mysql: typeof import("mysql2/promise");
  try {
    mysql = await import("mysql2/promise");
  } catch {
    throw new SrAccessError("other", "mysql2 is not installed in the scene package");
  }

  let conn: Awaited<ReturnType<typeof mysql.createConnection>>;
  try {
    conn = await mysql.createConnection({
      host: config.host,
      port: config.port,
      user: config.user,
      password: config.password,
      connectTimeout: 8000,
    });
  } catch (err) {
    throw classifySrError(err);
  }

  const qualified = (spec: EtlTableSpec) => `\`${spec.database}\`.\`${spec.table}\``;

  return {
    async describe(spec) {
      try {
        const [rows] = await conn.query(`DESC ${qualified(spec)}`);
        return (rows as Array<{ Field: string }>).map((r) => r.Field);
      } catch (err) {
        throw classifySrError(err);
      }
    },
    async query(spec, limit) {
      try {
        const sql = limit && limit > 0
          ? `SELECT * FROM ${qualified(spec)} LIMIT ${Math.floor(limit)}`
          : `SELECT * FROM ${qualified(spec)}`;
        const [rows] = await conn.query(sql);
        return rows as SrRow[];
      } catch (err) {
        throw classifySrError(err);
      }
    },
    async close() {
      await conn.end();
    },
  };
}
