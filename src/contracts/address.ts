/**
 * KnowledgeAddress — precise logical location of one maintenance/read unit
 * within a repository. Repository identity comes from request context or
 * KnowledgeRef, never inferred from path (K-04).
 *
 * Uniqueness is the address, not object_id alone (DataHub: URN + aspect name).
 * Same object_id may have many Aspect/Member files; duplicate addresses conflict.
 */

import type { ObjectIdentity } from "./identity.ts";
import { IngressError } from "./errors.ts";

export type AddressKind = "Entity" | "Aspect" | "Member" | "Relation" | "Record";

export interface KnowledgeAddress {
  readonly kind: AddressKind;
  readonly objectId: ObjectIdentity;
  readonly aspectName?: string;
  readonly memberKey?: string;
}

const SEP = "\u001f";

/** Canonical key for one maintenance unit. */
export function addressKey(address: KnowledgeAddress): string {
  return `${address.objectId}${SEP}${address.aspectName ?? ""}${SEP}${address.memberKey ?? ""}`;
}

export function isEntityBlob(address: KnowledgeAddress): boolean {
  return !address.aspectName && !address.memberKey;
}

/** PUT/REMOVE addresses: Entity blob, Aspect/Record, or Member. */
export function assertWritableAddress(address: KnowledgeAddress): void {
  const aspect = address.aspectName?.trim() ?? "";
  const member = address.memberKey?.trim() ?? "";
  switch (address.kind) {
    case "Entity":
    case "Relation":
      if (aspect || member) {
        throw new IngressError(
          "PRECONDITION_FAILED",
          `${address.kind} address cannot carry aspectName/memberKey`,
        );
      }
      return;
    case "Aspect":
    case "Record":
      if (!aspect || member) {
        throw new IngressError(
          "PRECONDITION_FAILED",
          `${address.kind} address requires aspectName and no memberKey`,
        );
      }
      return;
    case "Member":
      if (!aspect || !member) {
        throw new IngressError(
          "PRECONDITION_FAILED",
          "Member address requires aspectName and memberKey",
        );
      }
      return;
    default: {
      const _never: never = address.kind;
      throw new IngressError("PRECONDITION_FAILED", `unknown address kind ${String(_never)}`);
    }
  }
}
