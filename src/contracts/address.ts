/**
 * KnowledgeAddress — precise logical location of one maintenance/read unit
 * within a repository. Repository identity comes from request context or
 * KnowledgeRef, never inferred from path (K-04).
 */

import type { ObjectIdentity } from "./identity.ts";

export type AddressKind = "Entity" | "Aspect" | "Member" | "Relation" | "Record";

export interface KnowledgeAddress {
  readonly kind: AddressKind;
  readonly objectId: ObjectIdentity;
  readonly aspectName?: string;
  readonly memberKey?: string;
}
