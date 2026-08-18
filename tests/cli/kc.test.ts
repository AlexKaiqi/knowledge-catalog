import { afterEach, describe, expect, it } from "vitest";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { HELP, runCli } from "../../src/cli/main.ts";

const homes: string[] = [];
afterEach(() => {
  for (const dir of homes.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function home(): string {
  const dir = mkdtempSync(path.join(tmpdir(), "kc-cli-"));
  homes.push(dir);
  return dir;
}

function kc(h: string, ...args: string[]) {
  return runCli(["--home", h, ...args]);
}

function body(result: { status: number; stdout: string }): unknown {
  expect(result.status).toBe(0);
  return JSON.parse(result.stdout);
}

describe("kc CLI maps protocol verbs", () => {
  it("prints verb I/O on help", () => {
    const result = runCli(["help"]);
    expect(result.status).toBe(0);
    expect(result.stdout).toBe(HELP.endsWith("\n") ? HELP : `${HELP}\n`);
    expect(result.stdout).toContain("kc put");
    expect(result.stdout).toContain("kc read-release");
    expect(result.stdout).toContain("Output: ProvenanceTrace");
    expect(result.stdout).toContain("kc validate");
    expect(result.stdout).toContain("kc log");
  });

  it("walks put → read → append/stream → pin-view → promote → read-release", () => {
    const h = home();
    expect(kc(h, "init").status).toBe(0);
    const added = body(kc(h, "repo-add", "--repo", "kr://acme/public/core")) as { head: string };
    expect(added.head).toMatch(/^[0-9a-f]{40}$/);

    const put = body(kc(
      h, "put",
      "--command-id", "sync-1",
      "--repo", "kr://acme/public/core",
      "--object", "ETLTask:job-1",
      "--aspect", "io",
      "--value", JSON.stringify({ inputs: ["a"], outputs: ["b"] }),
      "--origin-kind", "SOURCE",
      "--source-ref", "csv://runs",
    )) as { result: { newCommit: string }; disposition: string };
    expect(put.disposition).toBe("APPLIED");
    const commit = put.result.newCommit;

    const replay = body(kc(
      h, "put",
      "--command-id", "sync-1",
      "--repo", "kr://acme/public/core",
      "--object", "ETLTask:job-1",
      "--aspect", "io",
      "--value", JSON.stringify({ inputs: ["a"], outputs: ["b"] }),
      "--origin-kind", "SOURCE",
      "--source-ref", "csv://runs",
    )) as { disposition: string };
    expect(replay.disposition).toBe("REPLAYED");

    const resolved = body(kc(h, "resolve", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--commit", commit)) as {
      status: string;
    };
    expect(resolved.status).toBe("RESOLVED");

    const read = body(kc(h, "read", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--commit", commit)) as {
      value: unknown;
    };
    expect(read.value).toEqual({ io: { inputs: ["a"], outputs: ["b"] } });

    const provenance = body(kc(h, "provenance", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--commit", commit)) as {
      chain: { originKind: string }[];
      value?: unknown;
    };
    expect(provenance.value).toBeUndefined();
    expect(provenance.chain[0]?.originKind).toBe("SOURCE");

    const appended = body(kc(
      h, "append",
      "--command-id", "run-1",
      "--repo", "kr://acme/public/core",
      "--stream", "runs",
      "--event-id", "evt-1",
      "--payload", JSON.stringify({ status: "ok" }),
    )) as { result: { cursor: string } };
    expect(appended.result.cursor).toBe("1");
    const slice = body(kc(h, "stream", "--repo", "kr://acme/public/core", "--stream", "runs")) as {
      records: { eventId: string }[];
    };
    expect(slice.records[0]?.eventId).toBe("evt-1");

    body(kc(
      h, "define-view",
      "--view", "agent",
      "--revision", "1",
      "--source", "kr://acme/public/core=refs/heads/main",
    ));
    body(kc(h, "promote", "--release", "stable", "--view", "agent"));

    const later = path.join(h, "later.json");
    writeFileSync(later, JSON.stringify({ inputs: ["changed"] }));
    body(kc(
      h, "put",
      "--command-id", "sync-2",
      "--repo", "kr://acme/public/core",
      "--object", "ETLTask:job-1",
      "--aspect", "io",
      "--file", later,
    ));

    const serving = body(kc(h, "read-release", "--release", "stable", "--object", "ETLTask:job-1")) as {
      value: unknown;
    }[];
    expect(serving).toHaveLength(1);
    expect(serving[0]?.value).toEqual({ io: { inputs: ["a"], outputs: ["b"] } });

    const live = body(kc(h, "read", "--repo", "kr://acme/public/core", "--object", "ETLTask:job-1", "--ref", "refs/heads/main")) as {
      value: unknown;
    };
    expect(live.value).toEqual({ io: { inputs: ["changed"] } });
  });

  it("returns protocol error codes as JSON", () => {
    const h = home();
    kc(h, "init");
    kc(h, "repo-add", "--repo", "kr://acme/public/core");
    const result = kc(h, "read", "--repo", "kr://acme/public/core", "--object", "missing", "--ref", "refs/heads/main");
    expect(result.status).toBe(1);
    expect(JSON.parse(result.stdout)).toMatchObject({ error: { code: "KNOWLEDGE_REF_UNRESOLVED" } });
  });

  it("same command-id with different PUT value is IDEMPOTENCY_CONFLICT", () => {
    const h = home();
    kc(h, "init");
    kc(h, "repo-add", "--repo", "kr://acme/public/core");
    body(kc(h, "put", "--command-id", "sync-1", "--repo", "kr://acme/public/core", "--object", "a", "--value", "1"));
    const conflict = kc(h, "put", "--command-id", "sync-1", "--repo", "kr://acme/public/core", "--object", "a", "--value", "2");
    expect(conflict.status).toBe(1);
    expect(JSON.parse(conflict.stdout)).toMatchObject({ error: { code: "IDEMPOTENCY_CONFLICT" } });
  });

  it("init namespace, object log/diff, catalog git history, and validate", () => {
    const h = home();
    body(kc(h, "init", "--namespace", "acme"));
    const status = body(kc(h, "status")) as {
      namespace: string;
      catalog: { repositoryId: string };
    };
    expect(status.namespace).toBe("acme");
    expect(status.catalog.repositoryId).toBe("kr://acme/catalog");

    const reserved = kc(h, "repo-add", "--repo", "kr://acme/catalog");
    expect(reserved.status).toBe(1);

    kc(h, "repo-add", "--repo", "kr://acme/public/core");
    const first = body(kc(
      h, "put",
      "--command-id", "v1",
      "--repo", "kr://acme/public/core",
      "--object", "policy/P-1",
      "--value", '{"version":1}',
    )) as { result: { newCommit: string } };
    const second = body(kc(
      h, "put",
      "--command-id", "v2",
      "--repo", "kr://acme/public/core",
      "--object", "policy/P-1",
      "--value", '{"version":2}',
    )) as { result: { newCommit: string } };

    const history = body(kc(
      h, "log",
      "--repo", "kr://acme/public/core",
      "--object", "policy/P-1",
      "--commit", second.result.newCommit,
    )) as { commit: string }[];
    expect(history[0]?.commit).toBe(second.result.newCommit);
    expect(history.some((item) => item.commit === first.result.newCommit)).toBe(true);

    const delta = body(kc(
      h, "diff",
      "--repo", "kr://acme/public/core",
      "--object", "policy/P-1",
      "--from", first.result.newCommit,
      "--to", second.result.newCommit,
    )) as { from?: { value: unknown }; to?: { value: unknown } };
    expect(delta.from?.value).toEqual({ version: 1 });
    expect(delta.to?.value).toEqual({ version: 2 });

    body(kc(h, "define-view", "--view", "agent", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"));
    body(kc(h, "promote", "--release", "stable", "--view", "agent"));
    const catalogLog = body(kc(h, "log", "--catalog", "--release", "stable")) as {
      commits: { message: string }[];
    };
    expect(catalogLog.commits.some((item) => item.message.startsWith("promote stable"))).toBe(true);
    expect(catalogLog.commits.some((item) => item.message.startsWith("define-view"))).toBe(false);
  });

  it("propose → record-validation → merge moves target Ref but not Release", () => {
    const h = home();
    kc(h, "init");
    kc(h, "repo-add", "--repo", "kr://acme/public/core");
    body(kc(h, "put", "--command-id", "seed", "--repo", "kr://acme/public/core", "--object", "policy/P-103", "--value", '{"v":1}'));
    body(kc(h, "define-view", "--view", "agent", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"));
    const published = body(kc(h, "promote", "--release", "stable", "--view", "agent")) as { generationId: string };

    const proposal = body(kc(
      h, "propose",
      "--proposal-id", "PR-1",
      "--repo", "kr://acme/public/core",
      "--target", "refs/heads/main",
      "--candidate", "refs/heads/candidates/PR-1",
      "--object", "policy/P-103",
      "--value", '{"v":2}',
    )) as { proposalId: string; candidateCommit: string };

    const preview = body(kc(h, "preview", "--proposal", "PR-1", "--view", "agent")) as { previewId: string };
    const structural = body(kc(h, "validate", "--preview", preview.previewId)) as {
      reportId: string;
      outcome: string;
      check: { issues: unknown[] };
    };
    expect(structural.outcome).toBe("PASSED");
    expect(structural.check.issues).toEqual([]);
    const validation = body(kc(
      h, "record-validation",
      "--preview", preview.previewId,
      "--suite", "S7",
      "--outcome", "PASSED",
    )) as { reportId: string };

    const merged = body(kc(
      h, "merge",
      "--proposal", proposal.proposalId,
      "--preview", preview.previewId,
      "--validation", validation.reportId,
    )) as { commitId: string };
    expect(merged.commitId).toBe(proposal.candidateCommit);

    const serving = body(kc(h, "read-release", "--release", "stable", "--object", "policy/P-103")) as { value: unknown }[];
    expect(serving[0]?.value).toEqual({ v: 1 });
    const live = body(kc(h, "read", "--repo", "kr://acme/public/core", "--object", "policy/P-103", "--ref", "refs/heads/main")) as {
      value: unknown;
    };
    expect(live.value).toEqual({ v: 2 });
    expect(published.generationId).toBeTruthy();
  });
});
