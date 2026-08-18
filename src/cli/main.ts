/**
 * kc — protocol verbs as commands. Output is JSON so input/output stay visible.
 *
 * Workspace is process glue (FileGit dirs + persisted Catalog/Writer state).
 * It is not a fourth write surface.
 */

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { IngressError } from "../contracts/errors.ts";
import type {
  AddressKind,
  AppendEntry,
  CommitChangeSet,
  KnowledgeAddress,
  Operation,
  OriginKind,
} from "../contracts/index.ts";
import { flagString, flagStrings, parseArgs, requireFlag, type FlagValue } from "./parse.ts";
import { CATALOG_OBJECT } from "../catalog/git-registry.ts";
import {
  addRepository,
  initWorkspace,
  openWorkspace,
  persistControl,
  type OpenWorkspace,
} from "./workspace.ts";

export const HELP = `kc — Knowledge Catalog CLI (protocol verbs)

Workspace (not protocol)
  kc init --home <dir> [--namespace acme]
  kc repo-add --home <dir> --repo <kr://...>
  kc status --home <dir>

Writer (mutates one Repository)
  kc put            --home <dir> --command-id <id> --repo <id> --object <id>
                    [--aspect <name>] [--member <key>] [--file <json>|--value <json>]
                    [--ref refs/heads/main] [--origin-kind SOURCE] [--source-ref <s>]
                    PUT one Address, then COMMIT. Output: CommitReceipt
  kc remove         same targeting flags without value. Output: CommitReceipt
  kc commit         --home <dir> --command-id <id> --changeset <file.json>
                    ChangeSet may omit base/expected (filled from current Ref). Output: CommitReceipt
  kc append         --home <dir> --command-id <id> --repo <id> --stream <name>
                    --event-id <id> [--payload <json>|--file <json>] [--cursor <n>]
                    Output: AppendReceipt

Reader (must name a version: --commit or --ref)
  kc resolve        --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    Output: Resolution (no body)
  kc read           same + optional --aspect / --include / --exclude
                    Output: KnowledgeValue
  kc provenance     same as resolve
                    Output: ProvenanceTrace { repository, commit, objectId, chain }
  kc stream         --home <dir> --repo <id> --stream <name>
                    Output: StreamSlice
  kc list           --home <dir> --repo <id> (--commit <id>|--ref <ref>)
                    Output: KnowledgeValue[]
  kc log            --home <dir> --repo <id> --object <id> (--commit <id>|--ref <ref>)
                    Output: ObjectRevision[]  (collapsed history)
  kc diff           --home <dir> --repo <id> --object <id> --from <commit> --to <commit>
                    Output: ObjectDiff
  kc log --catalog  --home <dir> [--release <name>|--view <id>|--object <id>]
                    Output: Catalog git history (promote / pin / define-view)

Catalog (combination + serving pointer; does not own object content)
  kc define-view    --home <dir> --view <id> --revision <n>
                    --source <repo>=<selector>   (repeatable)
                    Output: ViewDefinition (Catalog registry)
  kc pin-view       --home <dir> --view <id>
                    Output: ViewGeneration  {repo → commit} frozen now
  kc promote        --home <dir> --release <name>
                    (--view <id> | --generation <id>) [--expected <id>]
                    --view = pin now then CAS the Release (publish)
                    --generation = point at an already pinned generation
                    Output: { release, generationId }
  kc rollback       --home <dir> --release <name> --expected <id> --prior <id>
                    Output: { release, generationId }
  kc read-release   --home <dir> --release <name> --object <id>
                    Output: FederatedValue[]  (pinned generation, not live main)

Control Plane (content still goes through Writer; merge does not promote)
  kc propose        --home <dir> --proposal-id <id> --repo <id>
                    --target <ref> --candidate <ref>
                    PUT flags (--object --value/--file [--aspect]) or --changeset
                    Output: Proposal  (writes candidate Ref only)
  kc preview        --home <dir> --proposal <id> (--view <id>|--base-generation <id>)
                    Output: PreviewGeneration
  kc validate       --home <dir> --preview <id>
                    Structural check (mounted repos + commits exist), then records outcome
  kc record-validation --home <dir> --preview <id> --suite <rev> --outcome PASSED|FAILED
                    Records an external suite outcome; does not run tests
  kc merge          --home <dir> --proposal <id> --preview <id> --validation <id>
                    Fast-forwards target Ref. Release is unchanged.

Default --home is ./.kc
Writes require --command-id (retry = same id + same body; content change = new id).
Catalog registry is FileGit at <home>/repos/_catalog (kr://<namespace>/catalog).
Writer log: <home>/writer.json
`;

function jsonOut(value: unknown): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}

function parseJson(text: string, label: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    throw new Error(`${label} is not valid JSON`);
  }
}

function loadJsonFlag(flags: Readonly<Record<string, FlagValue>>, label: string): unknown | undefined {
  const file = flagString(flags, "file");
  const raw = flagString(flags, "value") ?? flagString(flags, "payload");
  if (file && raw) throw new Error("use only one of --file or --value/--payload");
  if (file) return parseJson(readFileSync(file, "utf8"), file);
  if (raw) return parseJson(raw, label);
  return undefined;
}

function addressFrom(flags: Readonly<Record<string, FlagValue>>): KnowledgeAddress {
  const objectId = requireFlag(flags, "object");
  const aspect = flagString(flags, "aspect");
  const member = flagString(flags, "member");
  if (member) {
    if (!aspect) throw new Error("Member address requires --aspect and --member");
    return { kind: "Member", objectId, aspectName: aspect, memberKey: member };
  }
  if (aspect) return { kind: "Aspect", objectId, aspectName: aspect };
  const kind = (flagString(flags, "kind") ?? "Entity") as AddressKind;
  return { kind, objectId };
}

function pinCommit(ws: OpenWorkspace, flags: Readonly<Record<string, FlagValue>>): {
  repositoryId: string;
  commitId: string;
} {
  const repositoryId = requireFlag(flags, "repo");
  const repo = ws.store.repos.get(repositoryId);
  if (!repo) throw new Error(`unknown repository ${repositoryId}`);
  const commit = flagString(flags, "commit");
  if (commit) return { repositoryId, commitId: commit };
  const ref = flagString(flags, "ref") ?? "refs/heads/main";
  const commitId = repo.getRef(ref);
  if (!commitId) throw new Error(`unresolved ref ${ref} in ${repositoryId}`);
  return { repositoryId, commitId };
}

function originFrom(flags: Readonly<Record<string, FlagValue>>) {
  const originKind = flagString(flags, "origin-kind") as OriginKind | undefined;
  const sourceRefs = flagStrings(flags, "source-ref");
  if (!originKind && sourceRefs.length === 0) return undefined;
  return {
    originKind: originKind ?? "SOURCE",
    sourceRefs: sourceRefs.length ? sourceRefs : undefined,
  };
}

function proposeOperations(flags: Readonly<Record<string, FlagValue>>): Operation[] {
  const file = flagString(flags, "changeset");
  if (file) {
    const raw = parseJson(readFileSync(file, "utf8"), file) as { operations?: Operation[] } | Operation[];
    const operations = Array.isArray(raw) ? raw : raw.operations;
    if (!operations?.length) throw new Error("changeset must include operations");
    return operations;
  }
  const value = loadJsonFlag(flags, "--value");
  if (value === undefined) throw new Error("propose requires --file/--value or --changeset");
  return [{ op: "PUT", address: addressFrom(flags), value, pathHint: flagString(flags, "path-hint") }];
}

function commitOne(
  ws: OpenWorkspace,
  flags: Readonly<Record<string, FlagValue>>,
  operations: readonly Operation[],
): unknown {
  const repositoryId = requireFlag(flags, "repo");
  const repo = ws.store.repos.get(repositoryId);
  if (!repo) throw new Error(`unknown repository ${repositoryId}`);
  const targetRef = flagString(flags, "ref") ?? "refs/heads/main";
  const commandId = requireFlag(flags, "command-id");
  return ws.writer.commitIntent(commandId, {
    targetRepository: repositoryId,
    targetRef,
    operations,
    message: flagString(flags, "message"),
    provenance: originFrom(flags),
  });
}

function handle(command: string, flags: Readonly<Record<string, FlagValue>>): unknown {
  const home = path.resolve(flagString(flags, "home") ?? ".kc");

  if (command === "help" || command === "--help" || command === "-h" || flags["help"] === true) return HELP;

  if (command === "init") {
    const namespace = flagString(flags, "namespace") ?? "local";
    initWorkspace(home, namespace);
    return { home, namespace, initialized: true };
  }

  if (command === "repo-add") {
    const ws = openWorkspace(home);
    const repositoryId = requireFlag(flags, "repo");
    const head = addRepository(ws, repositoryId);
    return { repositoryId, head };
  }

  if (command === "status") {
    const ws = openWorkspace(home);
    const catalog = ws.catalog.dumpState();
    return {
      home,
      repos: ws.file.repos.map((r) => {
        const repo = ws.store.repos.get(r.id);
        return {
          id: r.id,
          dir: r.dir,
          head: repo?.head("refs/heads/main"),
        };
      }),
      namespace: ws.file.namespace,
      catalog: {
        repositoryId: ws.catalogRegistry.repo.repositoryId,
        head: ws.catalogRegistry.repo.head("refs/heads/main"),
      },
      views: catalog.views,
      releases: catalog.releases,
      generations: catalog.generations,
    };
  }

  const ws = openWorkspace(home);

  switch (command) {
    case "put": {
      const value = loadJsonFlag(flags, "--value");
      if (value === undefined) throw new Error("put requires --file or --value");
      return commitOne(ws, flags, [{
        op: "PUT",
        address: addressFrom(flags),
        value,
        pathHint: flagString(flags, "path-hint"),
      }]);
    }
    case "remove":
      return commitOne(ws, flags, [{ op: "REMOVE", address: addressFrom(flags) }]);
    case "commit": {
      const file = requireFlag(flags, "changeset");
      const raw = parseJson(readFileSync(file, "utf8"), file) as Partial<CommitChangeSet>;
      if (!raw.targetRepository || !raw.operations) {
        throw new Error("changeset must include targetRepository and operations");
      }
      if (!ws.store.repos.get(raw.targetRepository)) {
        throw new Error(`unknown repository ${raw.targetRepository}`);
      }
      const targetRef = raw.targetRef ?? "refs/heads/main";
      return ws.writer.commitIntent(requireFlag(flags, "command-id"), {
        targetRepository: raw.targetRepository,
        targetRef,
        baseCommit: raw.baseCommit,
        expectedTargetCommit: raw.expectedTargetCommit,
        operations: raw.operations,
        message: raw.message,
        provenance: raw.provenance,
      });
    }
    case "append": {
      const payload = loadJsonFlag(flags, "--payload") ?? {};
      const entry: AppendEntry = {
        eventId: requireFlag(flags, "event-id"),
        eventType: flagString(flags, "event-type"),
        payload,
      };
      return ws.writer.appendIntent(requireFlag(flags, "command-id"), {
        targetRepository: requireFlag(flags, "repo"),
        streamRef: requireFlag(flags, "stream"),
        expectedCursor: flagString(flags, "cursor"),
        entries: [entry],
      });
    }
    case "resolve": {
      const { repositoryId, commitId } = pinCommit(ws, flags);
      return ws.reader.resolve({ repository: repositoryId, object: requireFlag(flags, "object") }, commitId);
    }
    case "read": {
      const { repositoryId, commitId } = pinCommit(ws, flags);
      const aspect = flagString(flags, "aspect");
      if (aspect) {
        return ws.reader.readAddress(repositoryId, addressFrom(flags), commitId);
      }
      const include = flagStrings(flags, "include");
      const exclude = flagStrings(flags, "exclude");
      const selector = include.length || exclude.length
        ? { include: include.length ? include : undefined, exclude: exclude.length ? exclude : undefined }
        : undefined;
      return ws.reader.read({ repository: repositoryId, object: requireFlag(flags, "object") }, commitId, selector);
    }
    case "provenance": {
      const { repositoryId, commitId } = pinCommit(ws, flags);
      return ws.reader.getProvenance({ repository: repositoryId, object: requireFlag(flags, "object") }, commitId);
    }
    case "stream":
      return ws.reader.readStream(requireFlag(flags, "repo"), requireFlag(flags, "stream"));
    case "list": {
      const { repositoryId, commitId } = pinCommit(ws, flags);
      return ws.reader.list(repositoryId, commitId);
    }
    case "log": {
      if (flags.catalog === true) {
        const objectId = flagString(flags, "release")
          ? CATALOG_OBJECT.release(requireFlag(flags, "release"))
          : flagString(flags, "view")
            ? CATALOG_OBJECT.view(requireFlag(flags, "view"))
            : flagString(flags, "object");
        return {
          repositoryId: ws.catalogRegistry.repo.repositoryId,
          commits: ws.catalogRegistry.history(Number(flagString(flags, "limit") ?? 20), objectId),
        };
      }
      const { repositoryId, commitId } = pinCommit(ws, flags);
      return ws.reader.log(
        repositoryId,
        requireFlag(flags, "object"),
        commitId,
        flagString(flags, "limit") ? Number(flagString(flags, "limit")) : undefined,
      );
    }
    case "diff": {
      const repositoryId = requireFlag(flags, "repo");
      return ws.reader.diff(
        repositoryId,
        requireFlag(flags, "object"),
        requireFlag(flags, "from"),
        requireFlag(flags, "to"),
      );
    }
    case "define-view": {
      const sources = flagStrings(flags, "source").map((item) => {
        const eq = item.indexOf("=");
        if (eq < 0) throw new Error(`--source must be repo=selector, got ${item}`);
        return { repository: item.slice(0, eq), selector: item.slice(eq + 1) };
      });
      if (!sources.length) throw new Error("define-view requires at least one --source repo=selector");
      const revision = Number(requireFlag(flags, "revision"));
      if (!Number.isFinite(revision)) throw new Error("--revision must be a number");
      return ws.catalog.defineView(requireFlag(flags, "view"), revision, sources);
    }
    case "pin-view":
      return ws.catalog.pinView(requireFlag(flags, "view"));
    case "promote": {
      const release = requireFlag(flags, "release");
      const viewId = flagString(flags, "view");
      const generationId = flagString(flags, "generation");
      if (viewId && generationId) throw new Error("use only one of --view or --generation");
      if (viewId) return ws.catalog.publish(release, viewId, flagString(flags, "expected"));
      if (!generationId) throw new Error("promote requires --view or --generation");
      ws.catalog.promote(release, flagString(flags, "expected"), generationId);
      return { release, generationId: ws.catalog.release(release) };
    }
    case "rollback": {
      const release = requireFlag(flags, "release");
      ws.catalog.rollback(release, requireFlag(flags, "expected"), requireFlag(flags, "prior"));
      return { release, generationId: ws.catalog.release(release) };
    }
    case "read-release":
      return ws.catalog.readRelease(requireFlag(flags, "release"), requireFlag(flags, "object"));
    case "propose": {
      const repositoryId = requireFlag(flags, "repo");
      const repo = ws.store.repos.get(repositoryId);
      if (!repo) throw new Error(`unknown repository ${repositoryId}`);
      const targetRef = flagString(flags, "target") ?? "refs/heads/main";
      const operations = proposeOperations(flags);
      const proposal = ws.controlPlane.propose({
        proposalId: requireFlag(flags, "proposal-id"),
        repositoryId,
        targetRef,
        candidateRef: requireFlag(flags, "candidate"),
        baseCommit: flagString(flags, "base") ?? repo.head(targetRef),
        operations,
        rationale: flagString(flags, "message"),
      });
      ws.control = {
        ...ws.control,
        proposals: { ...ws.control.proposals, [proposal.proposalId]: proposal },
      };
      persistControl(ws);
      return proposal;
    }
    case "preview": {
      const proposal = ws.control.proposals[requireFlag(flags, "proposal")];
      if (!proposal) throw new Error("unknown proposal; run propose first");
      const viewId = flagString(flags, "view");
      const base = viewId
        ? ws.catalog.pinView(viewId).generationId
        : requireFlag(flags, "base-generation");
      const preview = ws.controlPlane.createPreview(base, proposal);
      ws.control = {
        ...ws.control,
        previews: { ...ws.control.previews, [preview.previewId]: preview },
      };
      persistControl(ws);
      return preview;
    }
    case "validate": {
      const preview = ws.control.previews[requireFlag(flags, "preview")];
      if (!preview) throw new Error("unknown preview; run preview first");
      const report = ws.controlPlane.validateStructure(preview);
      ws.control = {
        ...ws.control,
        validations: { ...ws.control.validations, [report.reportId]: report },
      };
      persistControl(ws);
      return report;
    }
    case "record-validation": {
      const preview = ws.control.previews[requireFlag(flags, "preview")];
      if (!preview) throw new Error("unknown preview; run preview first");
      const outcome = requireFlag(flags, "outcome");
      if (outcome !== "PASSED" && outcome !== "FAILED") throw new Error("--outcome must be PASSED or FAILED");
      const report = ws.controlPlane.recordValidation(preview, requireFlag(flags, "suite"), outcome);
      ws.control = {
        ...ws.control,
        validations: { ...ws.control.validations, [report.reportId]: report },
      };
      persistControl(ws);
      return report;
    }
    case "merge": {
      const proposal = ws.control.proposals[requireFlag(flags, "proposal")];
      const preview = ws.control.previews[requireFlag(flags, "preview")];
      const validation = ws.control.validations[requireFlag(flags, "validation")];
      if (!proposal || !preview || !validation) {
        throw new Error("merge needs stored --proposal, --preview and --validation ids");
      }
      const commitId = ws.controlPlane.merge(proposal, preview, validation);
      return { commitId, note: "target Ref moved; Release unchanged — run promote --view to serve" };
    }
    default:
      throw new Error(`unknown command ${command}\n\n${HELP}`);
  }
}

export function runCli(argv: readonly string[]): { status: number; stdout: string } {
  try {
    const parsed = parseArgs(argv);
    const result = handle(parsed.command, parsed.flags);
    if (typeof result === "string") return { status: 0, stdout: result.endsWith("\n") ? result : `${result}\n` };
    return { status: 0, stdout: jsonOut(result) };
  } catch (error) {
    if (error instanceof IngressError) {
      return { status: 1, stdout: jsonOut({ error: { code: error.code, message: error.message } }) };
    }
    const message = error instanceof Error ? error.message : String(error);
    return { status: 1, stdout: jsonOut({ error: { message } }) };
  }
}

const invoked = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (invoked) {
  const { status, stdout } = runCli(process.argv.slice(2));
  process.stdout.write(stdout);
  process.exit(status);
}
