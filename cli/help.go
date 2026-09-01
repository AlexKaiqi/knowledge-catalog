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
  kc catalog show
  kc catalog audit
  kc catalog archive
  kc catalog repository list|register|archive
  kc catalog workspace list|show|define|retire|resolve|check

Knowledge consumption (Workspace resolves once per command)
  kc knowledge search --workspace <id> [query flags]
  kc knowledge read --workspace <id> --object <object-id>
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

const ConsumerHelp = `kc help consumer — consume knowledge at one fixed Workspace pin

  kc catalog workspace resolve --workspace <id> > pin.json
  kc knowledge search --workspace <id> --pin pin.json --query <text>
  kc knowledge read --workspace <id> --pin pin.json --object <object-id>
  kc knowledge provenance --workspace <id> --pin pin.json --object <object-id>
  kc resource access --workspace <id> --pin pin.json --object <descriptor-id> \
    --operation <name> --input <json>

Known object IDs go directly to read. Unknown objects go through search. If a
capability is unavailable, the command fails explicitly; it never enumerates the
Repository. Mounted knowledge is read with ordinary ls/find/rg/cat and is read-only.
Resource access resolves the descriptor at the same pin; operation/call come from
that Canonical declaration, while --input supplies only this invocation's payload.
`

const ProviderHelp = `kc help provider — publish through Writer

  # deployment operator, once per local authority
  kc local repository attach --repo <kr://...>
  # provider/Connector, always through KC Server
  kc writer head --repo <id>
  kc writer put --command-id <id> --repo <id> --object <object-id> --value <json>
  kc writer ingest --repo <id> --dir <drafts> --out <changeset.json>
  kc writer commit --command-id <id> --changeset <changeset.json>

Collectors remain outside KC and submit ChangeSets. Schema is versioned knowledge
under schema/*, not project configuration. Do not edit Repository storage directly.
`

const GovernorHelp = `kc help governor — compose, authorize, and govern

  kc catalog workspace define --workspace <id> --revision <n> --source <repo>=<selector>
  kc admin grant add --principal <id> --action knowledge.read --repo <repo>
  kc governance proposal create ...
  kc governance preview create ...
  kc governance preview validate ...
  kc governance proposal merge ...

Catalog composes Snapshot coordinates; it does not store knowledge. Grants use
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
