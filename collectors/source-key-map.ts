import type { ObjectIdentity } from "../src/contracts/identity.ts";
import type { SourceKey } from "./events.ts";
import { encodeSourceKey, objectIdFor } from "./keys.ts";

/** In-memory source key → object_id. Preview does not persist. */
export class SourceKeyMap {
  private readonly map = new Map<string, ObjectIdentity>();

  resolve(key: SourceKey): ObjectIdentity {
    const encoded = encodeSourceKey(key);
    const existing = this.map.get(encoded);
    if (existing) return existing;
    const objectId = objectIdFor(key);
    this.map.set(encoded, objectId);
    return objectId;
  }
}
