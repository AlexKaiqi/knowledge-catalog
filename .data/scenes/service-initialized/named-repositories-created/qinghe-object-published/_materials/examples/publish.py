#!/usr/bin/env python3
"""Publish the Qinghe example world to an already deployed Knowledge Catalog.

The plan is world.json next to this file. The script only talks to a running
Server (through the `kc` CLI) and, for the two connected repositories, to the
integrator's own lakeFS. It never starts services and is safe to re-run:
publication resumes after the last step the Server can verify (a Writer receipt
or an accepted Dataset revision), so an earlier raw tree is never replayed over
a later correction.

Environment:
  KC_SERVER_URL          Server address (required)
  KC_AS                  principal passed as --as (optional; otherwise the saved login)
  KC_BIN                 kc executable (default: kc)
  KC_STORE               managed store for `kc create --name` (optional)
  EXAMPLES_LAKEFS_URL    origin of the integrator lakeFS hosting media and store-ops
  EXAMPLES_LAKEFS_CREDENTIAL_FILE  one line `accessKey:secret` for that lakeFS

Usage:
  publish.py            publish or resume
  publish.py --plan     show the steps that would run, change nothing
"""

import base64
import json
import os
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

ROOT = os.path.dirname(os.path.abspath(__file__))


def fail(message):
    sys.stderr.write("publish: " + message + "\n")
    sys.exit(1)


def kc(*args, check=True):
    server = os.environ.get("KC_SERVER_URL")
    if not server:
        fail("KC_SERVER_URL is required")
    argv = [os.environ.get("KC_BIN", "kc"), "--server", server]
    if os.environ.get("KC_AS"):
        argv += ["--as", os.environ["KC_AS"]]
    argv += list(args)
    # Relative --dir values keep the publisher's local paths out of the
    # provenance that public readers see.
    done = subprocess.run(argv, cwd=ROOT, capture_output=True, text=True)
    try:
        value = json.loads(done.stdout) if done.stdout.strip() else {}
    except json.JSONDecodeError:
        value = {"raw": done.stdout}
    if check and done.returncode != 0:
        fail("kc %s failed: %s %s" % (" ".join(args), done.stdout.strip(), done.stderr.strip()))
    return done.returncode, value


class LakeFS:
    def __init__(self):
        origin = os.environ.get("EXAMPLES_LAKEFS_URL", "").rstrip("/")
        path = os.environ.get("EXAMPLES_LAKEFS_CREDENTIAL_FILE", "")
        if not origin or not path:
            fail("EXAMPLES_LAKEFS_URL and EXAMPLES_LAKEFS_CREDENTIAL_FILE are required for connected repositories")
        with open(path, encoding="utf-8") as handle:
            credential = handle.read().strip()
        self.origin = origin
        self.credential_file = path
        self.auth = "Basic " + base64.b64encode(credential.encode()).decode()

    def request(self, method, path, query=None, body=None, content_type=None):
        url = self.origin + "/api/v1" + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        req = urllib.request.Request(url, data=body, method=method)
        req.add_header("Authorization", self.auth)
        if content_type:
            req.add_header("Content-Type", content_type)
        try:
            with urllib.request.urlopen(req) as resp:
                return resp.status, resp.read()
        except urllib.error.HTTPError as err:
            return err.code, err.read()

    def read(self, repo, path):
        status, raw = self.request("GET", "/repositories/%s/refs/main/objects" % repo, {"path": path})
        return raw if status == 200 else None

    def upload(self, repo, path, raw):
        boundary = uuid.uuid4().hex
        body = (("--%s\r\nContent-Disposition: form-data; name=\"content\"; filename=\"%s\"\r\n"
                 "Content-Type: application/octet-stream\r\n\r\n") % (boundary, os.path.basename(path))).encode() + raw + ("\r\n--%s--\r\n" % boundary).encode()
        status, out = self.request("POST", "/repositories/%s/branches/main/objects" % repo, {"path": path}, body, "multipart/form-data; boundary=" + boundary)
        if status not in (200, 201):
            fail("lakeFS upload %s/%s: %s %s" % (repo, path, status, out[:200]))

    def commit(self, repo, message):
        body = json.dumps({"message": message, "metadata": {"source": "kc-examples"}}).encode()
        status, out = self.request("POST", "/repositories/%s/branches/main/commits" % repo, None, body, "application/json")
        if status not in (200, 201):
            fail("lakeFS commit %s: %s %s" % (repo, status, out[:200]))


def raw_files(step, lakefs):
    base = os.path.join(ROOT, step["dir"])
    changed = 0
    for folder, _, names in os.walk(base):
        for name in sorted(names):
            local = os.path.join(folder, name)
            rel = os.path.relpath(local, base).replace(os.sep, "/")
            with open(local, "rb") as handle:
                raw = handle.read()
            if lakefs.read(step["repository"], rel) == raw:
                continue
            lakefs.upload(step["repository"], rel, raw)
            changed += 1
    if changed:
        lakefs.commit(step["repository"], step.get("message", "kc examples"))
    return changed


def verifiable_done(step):
    if step["kind"] == "commit":
        return kc("writer", "receipt", "--command-id", step["commandId"], check=False)[0] == 0
    if step["kind"] == "define":
        return kc("dataset", "show", step["dataset"], "--revision", str(step["revision"]), check=False)[0] == 0
    return False


def provision(world, lakefs_factory):
    _, shown = kc("show")
    attached = {item.get("id") for item in shown.get("repositories", [])}
    for repo in world["repositories"]:
        rid = repo["id"]
        if rid in attached:
            continue
        if repo["provision"] == "managed":
            args = ["create", "--name", rid]
            if os.environ.get("KC_STORE"):
                args += ["--store", os.environ["KC_STORE"]]
        else:
            lakefs = lakefs_factory()
            args = ["create", "--url", lakefs.origin + "/" + rid, "--credential-file", lakefs.credential_file]
        kc(*args)
        kc("attach", "--repo", rid)
        print("repository %s attached" % rid)


def main():
    with open(os.path.join(ROOT, "world.json"), encoding="utf-8") as handle:
        world = json.load(handle)
    steps = world["steps"]
    resume = 0
    for index, step in enumerate(steps):
        if verifiable_done(step):
            resume = index + 1
    pending = steps[resume:]
    if "--plan" in sys.argv[1:]:
        print("resume at step %d of %d" % (resume, len(steps)))
        for step in pending:
            print("  %s %s" % (step["kind"], step.get("commandId") or step.get("dataset") or step["dir"]))
        return
    cache = {}

    def lakefs_factory():
        if "client" not in cache:
            cache["client"] = LakeFS()
        return cache["client"]

    provision(world, lakefs_factory)
    for step in pending:
        kind = step["kind"]
        if kind == "files":
            print("files %s: %d changed" % (step["dir"], raw_files(step, lakefs_factory())))
        elif kind == "commit":
            kc("writer", "commit", "--command-id", step["commandId"], "--repo", step["repository"], "--dir", step["dir"])
            print("commit %s" % step["commandId"])
        elif kind == "define":
            kc("dataset", "define", step["dataset"], "--revision", str(step["revision"]), "--file", step["recipe"])
            print("dataset %s r%d" % (step["dataset"], step["revision"]))
        else:
            fail("unknown step kind " + kind)
    print("published: %s" % ", ".join(world["datasets"]))


if __name__ == "__main__":
    main()
