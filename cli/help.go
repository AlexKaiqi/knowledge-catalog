package cli

import (
	"fmt"
	"strings"
)

// Help documents only the public grouped CLI. Internal operation names,
// maintenance SPIs, HTTP DTOs, and mount protocols are deliberately absent.
const Help = `kc — Knowledge Catalog CLI

Top-level verbs: help, serve, login, logout, whoami, pack.

Agent and human entry point
  kc help [consume|write|compose]
  kc serve --home <dir> --auth local|taihu|gitea [--listen <address>] [--rerank-model <model>] [--rerank-timeout <duration>]

Global
  Product commands require --server or KC_SERVER_URL, including local deployments.
  kc serve requires --auth local|taihu|gitea. kc login first reads GET /identity/v1/auth;
  default --mode follows the Server. local pairing uses --as; token pairing uses KC_AUTH_TOKEN.
  Only kc local and kc serve open --home directly. A Workspace pin is fixed for one command.

Host (never exposed by HTTP)
  kc local init
  kc local status
  kc local catalog attach
  kc local repository attach
  kc local store show
  kc local store set
  kc local grant bootstrap --principal <id>
  kc local system publish --driver gitea --dsn http://127.0.0.1:13001/kc/system
  kc local workspace overlay

Identity
  kc login --server <url> [--mode taihu|token|local] [--as <principal>] [--wait]
  kc logout --server <url>
  kc whoami

Catalog
  kc catalog list
  kc catalog show [<catalog>]
  kc catalog audit [<catalog>]
  kc catalog archive <catalog>
  kc catalog repo list|register|archive

Workspace
  kc workspace list
  kc workspace show <id>
  kc workspace define --workspace <id> --revision <n> --source <repository>
  kc workspace retire [<id>]
  kc workspace pin [<id>|--source]   (prints ResolvedWorkspace JSON; does not write Catalog)
  kc workspace check [<id>]

Pack (Client preprocess; not a Server write)
  kc pack --repo <id> --dir <drafts> [--out <changeset.json>] [--command-id is not required]
  Without --out, ChangeSet is on stdout. With --out, stdout is files/diagnostics only.

Writer (commit is the write primitive; put/remove are sugars)
  kc writer put --command-id <id> --repo <id> --object <object-id> --value <json>
  kc writer remove --command-id <id> --repo <id> --object <object-id>
  kc writer commit --command-id <id> --changeset <changeset.json>
  kc writer head --repo <id>
  kc writer receipt --command-id <id>

Knowledge (--workspace + --pin, or --repo; mixing is USAGE_INVALID)
  kc knowledge search --workspace <id> [query flags] [--limit n] [--continuation c]
  kc knowledge read --workspace <id> --object <object-id>
  kc knowledge read --repo <id> [--ref|--commit] --object <object-id>
  kc knowledge resolve --workspace <id> --object <object-id>
  kc knowledge relations --workspace <id> --object kc://<repository>/<object-id>
  kc knowledge provenance --workspace <id> --object <object-id>
  kc knowledge log --workspace <id> --object <object-id> [--limit n] [--continuation c]
  kc knowledge schema describe --workspace <id> [--object <object-id>]
  kc knowledge schema list --repo <id> [--ref|--commit] [--limit n] [--continuation c]
  kc knowledge binding show --workspace <id> --object <object-id> --aspect <name>
  kc knowledge access --workspace <id> --object <id> --aspect <name>
  kc knowledge access --workspace <id> --object <descriptor-id> \
    --operation <name> --input <json>
  schema list pages schema/* only; it is not an object LIST.
  knowledge access calls a wall-side runtime; it is not knowledge read.

Admin
  kc admin grant add --principal <id> --action <action[,action...]>
  kc admin grant list
  kc admin grant remove --id <rule-id>

Governance
  kc governance proposal create|merge
  kc governance preview create
  kc governance preview validate
  kc governance validation record

Operations
  kc operations projection describe|sync|notice
  kc operations access-spec describe
  kc operations hook add|list|remove
  kc operations gate add|list|remove
  kc operations audit access|trace|hitmap
  kc operations feedback record

Host-side checkout/sync/status and snapshot-export commands are not product
surfaces. Use kcfs for lazy files; add a typed streaming API before exporting.
Knowledge directories mounted by kcfs are read-only; the surrounding user
working directory remains writable and is accessed with ordinary shell tools.
There is no public Knowledge enumeration command and no scan fallback when
SEARCH, schema location, or relation location is unavailable.
`

const ConsumeHelp = `kc help consume — find knowledge, then read it at one fixed version

  kc login --server <url>
  kc whoami
  kc catalog list
  kc catalog show
  kc knowledge schema list --repo <id>
  kc workspace pin <id> > pin.json
  kc knowledge search --workspace <id> --pin pin.json --query <text>
  kc knowledge read --workspace <id> --pin pin.json --object <object-id>
  kc knowledge resolve --workspace <id> --pin pin.json --object <object-id>
  kc knowledge provenance --workspace <id> --pin pin.json --object <object-id>
  kc knowledge log --workspace <id> --pin pin.json --object <object-id>
  kc knowledge access --workspace <id> --pin pin.json --object <descriptor-id> \
    --operation <name> --input <json>

Start from the Server. You only need the Server URL, your identity, and what
you want to find. catalog list names visible Catalogs. catalog show lists
each knowledge source with its published title and summary when present, plus
named knowledge sets; it does not list objects or host paths. schema list is
the typed catalog of one knowledge source (schema/* only). Pick a named
knowledge set, pin it once (prints ResolvedWorkspace JSON, does not write
Catalog), and pass --pin to every later command. Known object IDs go directly
to read. Unknown objects go through search. If search returns
CAPABILITY_UNSATISFIED, that is not zero hits: the knowledge set is not
searchable yet, while exact read still works when you already have an object
id. The command never enumerates the knowledge source. Mounted knowledge is
read with ordinary ls/find/rg/cat and is read-only. knowledge access calls a
wall-side runtime using the same pin; operation/call come from that
declaration, while --input supplies only this invocation's payload.

To combine sources for this task without creating a named knowledge set:
  kc workspace pin --source <id> > pin.json
`

const WriteHelp = `kc help write — publish what you have, then read it back

  kc login --server <url>
  kc whoami
  kc pack --repo <id> --dir <drafts> --out <changeset.json>
  kc writer commit --command-id <id> --changeset <changeset.json>
  kc writer put --command-id <id> --repo <id> --object <object-id> --value <json>
  kc writer head --repo <id>
  kc knowledge read --repo <id> --object <object-id>
  kc knowledge provenance --repo <id> --object <object-id>
  kc knowledge schema list --repo <id>

A knowledge set is not a write prerequisite. You only need the Server URL, your
identity, your knowledge source id, and your drafts. pack with --out writes
the ChangeSet file; stdout reports files and diagnostics and does not publish.
Without --out, ChangeSet is on stdout. commit publishes. After commit, read
and provenance use the same --repo you just published to. Collectors remain
outside KC and submit ChangeSets.
Schema is versioned knowledge under schema/*, not project configuration.
Do not edit storage directly.
`

const ComposeHelp = `kc help compose — register sources, define a knowledge set, and grant

  kc login --server <url>
  kc whoami
  kc catalog repo register --repo <repository>
  kc workspace define --workspace <id> --revision <n> --source <repository>
  kc admin grant add --principal <id> --action writer.commit,writer.preview,knowledge.read --repo <repository>
  kc admin grant add --principal <id> --action catalog.read,workspace.resolve,workspace.consume --catalog <id>
  kc admin grant add --principal <id> --action knowledge.read,knowledge.search --repo <repository>

After a source is attached on the host, register it so this Catalog admits it
into recipes. Omitting the selector uses that source's published default.
Grant writers on their source, and consumers catalog plus knowledge access.
Catalog composes published sources; it does not store knowledge. Grants use
stable semantic actions, not CLI command names.
Consumers never run these commands.
`

func helpFor(topic string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(topic)) {
	case "":
		return Help, nil
	case "consume":
		return ConsumeHelp, nil
	case "write":
		return WriteHelp, nil
	case "compose":
		return ComposeHelp, nil
	default:
		return "", fmt.Errorf("unknown help topic %s; want consume, write, or compose", topic)
	}
}
