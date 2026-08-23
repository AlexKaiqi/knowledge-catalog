/**
 * Text helpers for the ctx.fs adapter: strict UTF-8 decoding (dsh-fs's
 * readText contract is "regular UTF-8 text or typed errors", never lossy
 * replacement characters) and the literal search/replace editText needs.
 * Framework-free so vfs.test.ts can exercise them directly.
 */

const BINARY_SAMPLE_BYTES = 8192;

export class NotTextError extends Error {
  constructor(displayPath: string, cause?: unknown) {
    super(`cannot read "${displayPath}": not valid UTF-8 text`);
    this.name = 'NotTextError';
    this.cause = cause;
  }
}

/** Rejects binary content (a null byte in the first 8KiB, the same cheap
 * heuristic most editors use) and invalid UTF-8 before ever handing a
 * replacement-character-mangled string to a caller. */
export function decodeStrictText(bytes: Uint8Array, displayPath: string): string {
  const sample = bytes.subarray(0, BINARY_SAMPLE_BYTES);
  if (sample.includes(0)) {
    throw new NotTextError(displayPath);
  }
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
  } catch (cause) {
    throw new NotTextError(displayPath, cause);
  }
}

export class EditNotFoundError extends Error {
  constructor(displayPath: string) {
    super(`cannot edit "${displayPath}": oldString not found`);
    this.name = 'EditNotFoundError';
  }
}

export class AmbiguousEditError extends Error {
  constructor(displayPath: string, matches: number) {
    super(`cannot edit "${displayPath}": oldString matches ${matches} locations; pass replaceAll or a longer unique string`);
    this.name = 'AmbiguousEditError';
  }
}

function countOccurrences(haystack: string, needle: string): number {
  if (needle.length === 0) return 0;
  let count = 0;
  let index = haystack.indexOf(needle);
  while (index !== -1) {
    count++;
    index = haystack.indexOf(needle, index + needle.length);
  }
  return count;
}

/** Literal (non-regex) search/replace, requiring exactly one match unless
 * replaceAll — the same discipline dsh's own edit tool documents. */
export function applyLiteralEdit(content: string, oldString: string, newString: string, replaceAll: boolean, displayPath: string): string {
  const matches = countOccurrences(content, oldString);
  if (matches === 0) {
    throw new EditNotFoundError(displayPath);
  }
  if (matches > 1 && !replaceAll) {
    throw new AmbiguousEditError(displayPath, matches);
  }
  return replaceAll ? content.split(oldString).join(newString) : content.replace(oldString, newString);
}
