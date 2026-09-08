package cli

import (
	"fmt"
	"sort"
	"strings"
)

// Help documents only the public grouped CLI. Internal operation names,
// maintenance SPIs, HTTP DTOs, and mount protocols are deliberately absent.
const Help = `kc — Knowledge Catalog CLI

Top-level groups: deployment, catalog, workspace, knowledge, writer, governance, admin, operations.
Standalone commands: help, serve, login, logout, whoami, pack.

Agent and human entry point
  kc help [consume|write|compose|<command group>]
  kc serve --config deployment.yaml [--listen <address>]

Global
  Business commands require --server or KC_SERVER_URL.
  Deployment commands read only their explicit --config. serve restores that deployment.
  pack and workspace overlay process client files without a Server or identity.
  kc login reads the Server auth contract; local pairing uses --as, token pairing uses credentials.
  One task keeps one Workspace pin; export --pin for later commands.

Operands
  Catalog and Workspace ids are positional (flags --catalog/--workspace still work).
  Knowledge objects use --object. Repositories use --repo.
  Knowledge commands choose --workspace, --source, --workspace-file, --pin, or --repo.
  A named Workspace may carry its fixed --pin; incompatible bases are USAGE_INVALID.
  pack and workspace pin write a document with --out; stdout is then a receipt.

Deployment (operator commands; never exposed by HTTP)
  kc deployment init --config deployment.yaml
  kc deployment status --config deployment.yaml
  kc deployment system publish --config deployment.yaml
  kc deployment identity migrate --config deployment.yaml --file migration.json
    Explicit operator migration while Server is stopped; moves remaining legacy grants only.
  init explicitly initializes Catalog refs and durable control state.
  status restores existing authority; it does not create missing Catalogs.

Identity
  kc login --server <url> [--mode taihu|token|local] [--as <principal>] [--wait]
  kc logout --server <url>
  kc whoami
  kc admission show
  kc admission request
    Explicitly applies the deployment's first-use policy once; login grants no rights.
  kc catalog repo create --name <name> [--store <allowed-store>]
  kc catalog repo list --mine

Catalog
  kc catalog list
  kc catalog show [<catalog>]
  kc catalog audit [<catalog>]
  kc catalog archive <catalog>
  kc catalog repo list|attach|archive
  kc catalog repo create --name <name> [--store <allowed-store>] [--catalog <id>]
  kc catalog repo list --mine [--repo <id>]
  kc catalog repo connect --catalog <id> --repo <id> --url <gitea-repo-url> --credential-file <path>
  kc catalog repo connection show|check --repo <id>
  kc catalog repo connection rotate --repo <id> --credential-file <path>
    Connect validates an existing source at an approved provider without initializing it.
    Rotation verifies the same authority before replacing its private credential.
  kc catalog repo share list --repo <id>
  kc catalog repo share add --repo <id> --principal <username> --action <actions>
  kc catalog repo share remove --repo <id> --id <share-id>
  kc catalog repo create --catalog <id> --repo <id> --command-id <id>
  create allocates a platform-managed source under deployment policy.
  An explicit --catalog is required; creation does not discover Catalogs.

Workspace
  kc workspace list
  kc workspace show <id>
  kc workspace define <id> --revision <n> --source <repository>
  kc workspace retire [<id>]
  kc workspace pin [<id>|--source <repo>|--file recipe.yaml] [--out pin.json]
  kc workspace check [<id>]
  kc workspace overlay --file recipe.yaml --overlay overlay.yaml [--out merged.yaml]
  Without --out, pin prints ResolvedWorkspace JSON and does not write Catalog.
  With --out, the pin document is the file; stdout is workspaceId, pinId, and out.

Catalog discovery search
  kc knowledge search --catalog <id> --query <text>
  Uses the configured discoveryWorkspaceId, resolves its fixed pin, then runs SEARCH.
  Requires catalog.read; returned bodies still require current repository knowledge.read.
  Explicit workspace/repository/pin/source input retains its own consumption contract.

Pack (Client preprocess; not a Server write)
  kc pack --repo <id> --dir <drafts> [--base <commit>] [--out <changeset.json>]
  Without --out, ChangeSet is on stdout. With --out, stdout is files/diagnostics only.
  Packing does not contact the Server; use an explicit --base when fixing the write basis.

Writer (commit is the write primitive; put/remove are sugars)
  kc writer put --command-id <id> --repo <id> --object <object-id> --value <json>
  kc writer remove --command-id <id> --repo <id> --object <object-id>
  kc writer commit --command-id <id> --changeset <changeset.json>
  kc writer head --repo <id>
  kc writer receipt --command-id <id>

Knowledge (named, temporary, or replayed task basis; incompatible bases are USAGE_INVALID)
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
  kc knowledge invoke --workspace <id> --object <descriptor-id> \
    --operation <name> --input <json>
  schema list pages schema/* only; it is not an object LIST.
  knowledge access hydrates a Binding through a wall-side runtime; it is not knowledge read.
  knowledge invoke calls one ResourceDescriptor operation; it is not knowledge access.

Admin
  kc admin grant add --principal <id> --action <action[,action...]>
  kc admin grant list
  kc admin grant remove --id <rule-id>

Governance
  kc governance proposal create|merge
  kc governance preview create
  kc governance preview validate
  kc governance validation record

Operations — projection
  kc operations projection describe|sync|notice
  kc operations access-spec describe
Operations — outbound
  kc operations hook add|list|remove
  kc operations gate add|list|remove
Operations — evidence
  kc operations audit access|trace|hitmap
  kc operations feedback record

HTTP-only (no kc command)
  POST /knowledge/v1/search:rerank
  POST /knowledge/v1/rerank
  POST /operations/v1/retrieval-log:query
  POST /operations/v1/retrieval-training:query
  POST /operations/v1/refine-log:query
  POST /operations/v1/rerank-training:query
CLI-only
  catalog audit --layer (HTTP audit has no --layer switch)
Not provided (see MVP_ACCEPTANCE; not current CLI)
  knowledge search --catalog / Catalog discoveryWorkspaceId
  delivery-chain stages after repository-read stripping (unselected)

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
  kc workspace pin <id> --out pin.json
  kc knowledge search --workspace <id> --pin pin.json --query <text>
  kc knowledge read --workspace <id> --pin pin.json --object <object-id>
  kc knowledge resolve --workspace <id> --pin pin.json --object <object-id>
  kc knowledge provenance --workspace <id> --pin pin.json --object <object-id>
  kc knowledge log --workspace <id> --pin pin.json --object <object-id>
  kc knowledge access --workspace <id> --pin pin.json --object <id> --aspect <name>
  kc knowledge invoke --workspace <id> --pin pin.json --object <descriptor-id> \
    --operation <name> --input <json>

Minimum grants: catalog.read on the Catalog; workspace.resolve and
workspace.consume on the named knowledge set; knowledge.schema.read,
knowledge.search, and knowledge.read on each source you browse, search, or
read. resource.access is required for knowledge access and knowledge invoke.

Start from the Server. You only need the Server URL, your identity, and what
you want to find. catalog list names visible Catalogs. catalog show lists
each knowledge source with its published title and summary when present, plus
named knowledge sets; it does not list objects or host paths. schema list is
the typed catalog of one knowledge source (schema/* only). Pick a named
knowledge set, pin it once (--out writes the pin file; stdout is a receipt
and does not write Catalog), and pass --pin to every later command. Known
object IDs go directly to read. Unknown objects go through search. If search
returns CAPABILITY_UNSATISFIED, that is not zero hits: the knowledge set is
not searchable yet, while exact read still works when you already have an
object id. The command never enumerates the knowledge source. Mounted
knowledge is read with ordinary ls/find/rg/cat and is read-only. knowledge
access hydrates a Binding through a wall-side runtime using the same pin.
knowledge invoke calls one declared ResourceDescriptor operation; --input is
only this invocation's payload.

To combine sources for this task without creating a named knowledge set:
  kc workspace pin --source <id> --out pin.json
  kc workspace pin --file recipe.yaml --out pin.json
  kc knowledge read --pin pin.json --object <object-id>
`

const WriteHelp = `kc help write — publish what you have, then read it back

  kc login --server <url>
  kc whoami
  kc catalog repo create --catalog <catalog-id> --repo <source-id> --command-id <create-id>
  kc pack --repo <id> --dir <drafts> --out <changeset.json>
  kc writer commit --command-id <id> --changeset <changeset.json>
  kc writer put --command-id <id> --repo <id> --object <object-id> --value <json>
  kc writer head --repo <id>
  kc knowledge read --repo <id> --object <object-id>
  kc knowledge provenance --repo <id> --object <object-id>
  kc knowledge schema list --repo <id>

Minimum grants: writer.commit on the source; knowledge.read to read back;
knowledge.schema.read if you browse schemas. Client pack needs no writer.preview grant.

For a new platform-managed source, use --name and optionally --store/--catalog.
Server generates retry coordinates and returns the management URL. Retry the
same name and Store to recover the same allocation. --mine finds your saved
management addresses. The explicit --repo/--command-id form remains available
for protocol automation. Both forms require catalog.repositories.create and
apply only the deployment's approved creator policy; neither adopts a foreign source.

A knowledge set is not a write prerequisite. You only need the Server URL, your
identity, your knowledge source id, and your drafts. pack with --out writes
the ChangeSet file; stdout reports files and diagnostics and does not publish.
Without --out, ChangeSet is on stdout. commit publishes. After commit, read
and provenance use the same --repo you just published to. Collectors remain outside KC and submit ChangeSets.
Schema is versioned knowledge under schema/*, not project configuration.
Do not edit storage directly.
`

const ComposeHelp = `kc help compose — attach sources, define a knowledge set, and grant

  kc login --server <url>
  kc whoami
  kc catalog repo attach --repo <repository>
  kc workspace define <id> --revision <n> --source <repository>
  kc admin grant add --principal <id> --action writer.commit,writer.preview,knowledge.read --repo <repository>
  kc admin grant add --principal <id> --action catalog.read,workspace.resolve,workspace.consume --catalog <id>
  kc admin grant add --principal <id> --action knowledge.read,knowledge.search,knowledge.schema.read --repo <repository>

Minimum grants to issue those commands: catalog.repositories.manage,
workspace.manage, and admin.grants.manage.

An operator configures the existing Snapshot binding. attach validates that
authority and admits it to this Catalog in one operation. Omitting the selector uses that source's published default.
Grant writers on their source, and consumers catalog plus knowledge actions.
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
		prefix := strings.ToLower(strings.TrimSpace(topic))
		paths := []string{}
		for path := range cliSurface {
			if path == prefix || strings.HasPrefix(path, prefix+" ") {
				paths = append(paths, path)
			}
		}
		if prefix == "serve" {
			return "kc serve --config deployment.yaml [--listen <address>]\n", nil
		}
		if len(paths) > 0 {
			sort.Strings(paths)
			lines := []string{"kc help " + prefix, ""}
			for _, path := range paths {
				usage := "  kc " + path
				found := false
				for _, line := range strings.Split(Help, "\n") {
					if strings.HasPrefix(line, usage+" ") || line == usage {
						lines = append(lines, line)
						found = true
					}
				}
				if !found {
					lines = append(lines, usage)
				}
			}
			return strings.Join(lines, "\n") + "\n", nil
		}
		return "", fmt.Errorf("unknown help topic %s; want consume, write, or compose, or a command group", topic)
	}
}
