/**
 * Maps kc's protocol error codes (kernel.ErrorCode) and this package's own
 * text errors onto dsh-fs's FsErrorCode vocabulary. dsh-fs owns that
 * vocabulary so backends agree on codes instead of inventing message
 * strings (see @deepseek-ai/dsh-fs's types.ts) — this is the one seam where
 * kc's codes cross into it.
 */

import { FsError, type FsErrorCode } from '@deepseek-ai/dsh-fs';
import { LoomError } from './client.js';
import { AmbiguousEditError, EditNotFoundError, NotTextError } from './text.js';

/** kc codes that mean "the path/object does not exist at this coordinate". */
const NOT_FOUND_CODES = new Set(['KNOWLEDGE_REF_UNRESOLVED', 'VERSION_UNRESOLVED']);

function codeForLoomError(err: LoomError): FsErrorCode {
  switch (err.code) {
    case 'NON_FAST_FORWARD':
      return 'FS_STALE_VERSION';
    case 'FORBIDDEN':
      return 'FS_PERMISSION_DENIED';
    default:
      if (NOT_FOUND_CODES.has(err.code)) return 'FS_NOT_FOUND';
      // USAGE_INVALID covers both "malformed request" and "no mount owns this
      // path" (RouteMount's refusal) — both are honestly "not found" from a
      // filesystem consumer's point of view, not a request-shape bug on this
      // side, since the path came from a prior list()/read() or the caller.
      if (err.code === 'USAGE_INVALID') return 'FS_NOT_FOUND';
      return 'FS_IO_ERROR';
  }
}

/** Wraps any error raised while talking to kc serve into the FsError this
 * dsh-fs expects at every abstract method boundary. Already-typed FsErrors
 * (from a deeper call) pass through unchanged. */
export function toFsError(err: unknown, operation: string, displayPath: string): FsError {
  if (err instanceof FsError) return err;
  if (err instanceof LoomError) {
    return new FsError(`cannot ${operation} "${displayPath}": ${err.message}`, codeForLoomError(err), { cause: err });
  }
  if (err instanceof NotTextError) {
    return new FsError(err.message, 'FS_NOT_TEXT', { cause: err });
  }
  if (err instanceof EditNotFoundError) {
    return new FsError(err.message, 'FS_EDIT_NOT_FOUND', { cause: err });
  }
  if (err instanceof AmbiguousEditError) {
    return new FsError(err.message, 'FS_AMBIGUOUS_EDIT', { cause: err });
  }
  if (err instanceof DOMException && err.name === 'AbortError') {
    return new FsError(`${operation} aborted`, 'FS_ABORTED', { cause: err });
  }
  const message = err instanceof Error ? err.message : String(err);
  return new FsError(`cannot ${operation} "${displayPath}": ${message}`, 'FS_IO_ERROR', { cause: err });
}

/** True when a LoomError is the specific "does not exist" case, so a caller
 * can branch to absence instead of throwing (stat/lstat return undefined,
 * they do not throw FS_NOT_FOUND). */
export function isMissing(err: unknown): boolean {
  return err instanceof LoomError && (NOT_FOUND_CODES.has(err.code) || err.code === 'USAGE_INVALID');
}
