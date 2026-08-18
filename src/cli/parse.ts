export type FlagValue = string | boolean | string[];

export interface ParsedArgs {
  readonly command: string;
  readonly flags: Readonly<Record<string, FlagValue>>;
}

function setFlag(flags: Record<string, FlagValue>, name: string, value: string | boolean): void {
  const existing = flags[name];
  if (existing === undefined) flags[name] = value;
  else if (Array.isArray(existing)) existing.push(String(value));
  else flags[name] = [String(existing), String(value)];
}

/** Parse `kc [--flag value] <command> [--flag value]...`. First non-flag is the command. */
export function parseArgs(argv: readonly string[]): ParsedArgs {
  const flags: Record<string, FlagValue> = {};
  let command: string | undefined;
  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];
    if (!token) continue;
    if (token.startsWith("--")) {
      const raw = token.slice(2);
      const eq = raw.indexOf("=");
      if (eq >= 0) {
        setFlag(flags, raw.slice(0, eq), raw.slice(eq + 1));
        continue;
      }
      const next = argv[i + 1];
      if (next && !next.startsWith("--")) {
        setFlag(flags, raw, next);
        i += 1;
      } else {
        setFlag(flags, raw, true);
      }
      continue;
    }
    if (command) throw new Error(`unexpected argument ${token}`);
    command = token;
  }
  return { command: command ?? "help", flags };
}

export function flagString(flags: Readonly<Record<string, FlagValue>>, name: string): string | undefined {
  const value = flags[name];
  if (value === undefined || typeof value === "boolean") return undefined;
  if (Array.isArray(value)) return value[value.length - 1];
  return value;
}

export function flagStrings(flags: Readonly<Record<string, FlagValue>>, name: string): string[] {
  const value = flags[name];
  if (value === undefined || typeof value === "boolean") return [];
  return Array.isArray(value) ? value : [value];
}

export function requireFlag(flags: Readonly<Record<string, FlagValue>>, name: string): string {
  const value = flagString(flags, name);
  if (!value) throw new Error(`missing --${name}`);
  return value;
}
