package cli

import (
	"fmt"
	"strings"
)

// Help documents only the public grouped CLI. Internal operation names,
// maintenance SPIs, HTTP DTOs, and mount protocols are deliberately absent.
const Help = `kc — Knowledge Catalog CLI

Agent and human entry point
  kc help [consumer|provider|governor]
  kc serve --home <dir> [--listen <address>] [--rerank-model <model>] [--rerank-timeout <duration>]

Local deployment (never exposed by HTTP)
  kc local init
  kc local status
  kc local catalog attach
  kc local repository attach
  kc local store show
  kc local store set
  kc local grant bootstrap --principal <id>
  kc local system publish --driver gitea --dsn http://127.0.0.1:13001/kc/system
  kc local workspace overlay

Identity and grants
  kc login --server <url> [--mode taihu|token] [--token <t>] [--wait]
  kc logout --server <url>
  kc identity whoami
  kc admin grant add --principal <id> --action <action[,action...]>
  kc admin grant list
  kc admin grant remove --id <rule-id>

Catalog composition
  kc catalog list
  kc catalog show
  kc catalog audit
  kc catalog archive
  kc catalog repository list|register|archive
  kc catalog workspace list|show|define|retire|resolve|check

Knowledge consumption (a knowledge set is resolved once per command)
  kc knowledge search --workspace <id> [query flags]
  kc knowledge read --workspace <id> --object <object-id>
  kc knowledge read --repo <id> [--ref|--commit] --object <object-id>
  kc knowledge relations --workspace <id> --object kc://<repository>/<object-id>
  kc knowledge provenance --workspace <id> --object <object-id>
  kc knowledge log --workspace <id> --object <object-id>
  kc knowledge schema describe --workspace <id> [--object <object-id>]
  kc knowledge schema browse --repo <id> [--ref|--commit] [--limit n] [--continuation c]
  kc knowledge binding resolve --workspace <id> --object <object-id> --aspect <name>

Writing and governance
  kc writer put|remove|commit|ingest|head|receipt
  kc governance proposal create|merge
  kc governance preview create
  kc governance preview validate
  kc governance validation record

Operations
  kc operations projection describe|sync
  kc operations access describe
  kc operations hook add|list|remove
  kc operations gate add|list|remove
  kc operations audit access|trace|hitmap
  kc operations feedback record
  kc resource access --workspace <id> --object <id> --aspect <name>
  kc resource access --workspace <id> --object <descriptor-id> \
    --operation <name> --input <json>

Global behavior
  Product commands require --server or KC_SERVER_URL, including local deployments.
  Only kc local and kc serve open --home directly. A Workspace pin is fixed for one command.
  Host-side checkout/sync/status and snapshot-export commands are not product
  surfaces. Use kcfs for lazy files; add a typed streaming API before exporting.
  Knowledge directories mounted by kcfs are read-only; the surrounding user
  working directory remains writable and is accessed with ordinary shell tools.
  There is no public Knowledge enumeration command and no scan fallback when
  SEARCH, schema location, or relation location is unavailable.
`

const ConsumerHelp = `kc help consumer — find knowledge, then read it at one fixed version

  kc identity whoami
  kc catalog list
  kc catalog show
  kc knowledge schema browse --repo <id>
  kc catalog workspace resolve --workspace <id> > pin.json
  kc knowledge search --workspace <id> --pin pin.json --query <text>
  kc knowledge read --workspace <id> --pin pin.json --object <object-id>
  kc knowledge provenance --workspace <id> --pin pin.json --object <object-id>
  kc knowledge log --workspace <id> --pin pin.json --object <object-id>
  kc resource access --workspace <id> --pin pin.json --object <descriptor-id> \
    --operation <name> --input <json>

Start from the Server. You only need the Server URL, your identity, and what
you want to find. catalog list/show are the inventory; they name knowledge
sets and knowledge sources, not host paths or storage refs. schema browse is
the typed catalog of one knowledge source. Pick a named knowledge set, resolve
it once, and pass --pin to every later command. Known object IDs go directly
to read. Unknown objects go through search. If search returns
CAPABILITY_UNSATISFIED, that is not zero hits: the knowledge set is not
searchable yet, while exact read still works when you already have an object
id. The command never enumerates the knowledge source. Mounted knowledge is
read with ordinary ls/find/rg/cat and is read-only. Resource access uses the
same pin; operation/call come from that declaration, while --input supplies
only this invocation's payload.

To combine sources for this task without creating a named knowledge set:
  kc catalog workspace resolve --source <id> > pin.json
`

const ProviderHelp = `kc help provider — publish what you have, then read it back

  kc identity whoami
  kc writer ingest --repo <id> --dir <drafts> --out <changeset.json>
  kc writer commit --command-id <id> --changeset <changeset.json>
  kc writer put --command-id <id> --repo <id> --object <object-id> --value <json>
  kc writer head --repo <id>
  kc knowledge read --repo <id> --object <object-id>
  kc knowledge provenance --repo <id> --object <object-id>
  kc knowledge schema browse --repo <id>

A knowledge set is not a write prerequisite. You only need the Server URL, your
identity, your knowledge source id, and your drafts. ingest with --out writes
the ChangeSet file; stdout reports files and diagnostics and does not publish.
commit publishes. After commit, read and provenance use the same --repo you
just published to. Collectors remain outside KC and submit ChangeSets.
Schema is versioned knowledge under schema/*, not project configuration.
Do not edit storage directly.
`

const GovernorHelp = `kc help governor — compose, authorize, and make knowledge consumable

  kc catalog workspace define --workspace <id> --revision <n> --source <repository>
  kc admin grant add --principal <id> --action writer.commit,writer.preview,knowledge.read --repo <repository>
  kc admin grant add --principal <id> --action catalog.read,workspace.resolve,workspace.consume --catalog <id>
  kc admin grant add --principal <id> --action knowledge.read,knowledge.search --repo <repository>
  kc operations projection sync --repo <repository>
  kc governance proposal create ...
  kc governance preview create ...
  kc governance preview validate ...
  kc governance proposal merge ...
  kc catalog audit

After a source is published, define the named knowledge set consumers will
discover. Omitting the selector uses that source's published default. Grant
providers write/read on their source, and consumers catalog plus knowledge
access. Sync the search projection before consumers can SEARCH; this is not a
write and is not required for exact READ. Consumers never run these commands.
Catalog composes published sources; it does not store knowledge. Grants use
stable semantic actions, not CLI command names.
`

func helpFor(topic string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(topic)) {
	case "":
		return Help, nil
	case "consumer":
		return ConsumerHelp, nil
	case "provider":
		return ProviderHelp, nil
	case "governor":
		return GovernorHelp, nil
	default:
		return "", fmt.Errorf("unknown help topic %s; want consumer, provider, or governor", topic)
	}
}
