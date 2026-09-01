package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/knowledge/writer"
	"kc/snapshot"
)

// Write surface verbs. COMMIT and PROPOSAL compile Knowledge changes onto one
// Snapshot TreeStore; the algebra is only PUT and REMOVE. Dynamic State/Stream
// values are observations, not Writer surfaces.

func writeVerbs() map[string]command {
	return map[string]command{
		"put":         {stage: stageGoverned, run: verbPut},
		"remove":      {stage: stageGoverned, run: verbRemove},
		"commit":      {stage: stageGoverned, run: verbCommit},
		"ingest":      {stage: stageGoverned, run: verbIngest},
		"receipt":     {stage: stageGoverned, run: verbReceipt},
		"writer-head": {stage: stageGoverned, run: verbWriterHead},
	}
}

func verbWriterHead(cx *invocation) (any, error) {
	repositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := requireRepo(cx.WS, repositoryID)
	if err != nil {
		return nil, err
	}
	ref := cx.targetRef("ref")
	commit, err := repo.Head(ref)
	if err != nil {
		return nil, err
	}
	return map[string]any{"repository": repositoryID, "commit": commit}, nil
}

func verbPut(cx *invocation) (any, error) {
	value, ok, err := loadJSONFlag(cx.Flags, "--value")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("put requires --file or --value")
	}
	op, err := writeOperation(cx.Flags, knowledge.OpPut, value)
	if err != nil {
		return nil, err
	}
	return commitOne(cx, []knowledge.Operation{op})
}

func verbRemove(cx *invocation) (any, error) {
	op, err := writeOperation(cx.Flags, knowledge.OpRemove, nil)
	if err != nil {
		return nil, err
	}
	return commitOne(cx, []knowledge.Operation{op})
}

// commitOne is the single-operation COMMIT path behind put and remove.
func commitOne(cx *invocation, operations []knowledge.Operation) (any, error) {
	setTelemetryChangeCounts(cx.Observation, operations)
	rawRepositoryID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	if _, isCatalog := cx.WS.Catalogs[rawRepositoryID]; isCatalog {
		return nil, kernel.Fail(kernel.ErrTargetRepositoryDenied, "catalog %s is not a Snapshot Repository", rawRepositoryID)
	}
	if _, err := requireRepo(cx.WS, rawRepositoryID); err != nil {
		return nil, err
	}
	repositoryID := kernel.RepositoryID(rawRepositoryID)
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	return cx.WS.Writer.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository:     repositoryID,
		TargetRef:            cx.targetRef("ref"),
		BaseCommit:           kernel.CommitID(cx.flag("base")),
		ExpectedTargetCommit: kernel.CommitID(cx.flag("expected")),
		Operations:           operations,
		Message:              cx.flag("message"),
		Provenance:           originFrom(cx.Flags),
	})
}

// verbCommit applies a prepared ChangeSet, which is how an inbound connector
// mirrors an external authority: it previews, then hands the file to COMMIT.
func verbCommit(cx *invocation) (any, error) {
	if workspaceIDOf(cx.Flags) != "" {
		if cx.flag("changeset") != "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "commit --workspace cannot be combined with --changeset")
		}
		return commitWorkspace(cx)
	}
	file := cx.flag("changeset")
	payload := cx.flag("payload")
	if file != "" && payload != "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "use only one of --changeset or typed payload")
	}
	var body []byte
	var err error
	label := file
	if payload != "" {
		body = []byte(payload)
		label = "typed commit payload"
	} else {
		if file == "" {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "missing --changeset")
		}
		body, err = os.ReadFile(file)
		if err != nil {
			return nil, err
		}
	}
	raw, err := decodeChangeSet(body, label)
	if err != nil {
		return nil, err
	}
	setTelemetryChangeCounts(cx.Observation, raw.Operations)
	if _, isCatalog := cx.WS.Catalogs[string(raw.TargetRepository)]; isCatalog {
		return nil, kernel.Fail(kernel.ErrTargetRepositoryDenied, "catalog %s is not a Snapshot Repository", raw.TargetRepository)
	}
	if _, err := requireRepo(cx.WS, string(raw.TargetRepository)); err != nil {
		return nil, err
	}
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	return cx.WS.Writer.CommitIntent(commandID, writer.CommitIntent{
		TargetRepository:     raw.TargetRepository,
		TargetRef:            snapshot.RefOrDefault(raw.TargetRef),
		BaseCommit:           raw.BaseCommit,
		ExpectedTargetCommit: raw.ExpectedTargetCommit,
		Operations:           raw.Operations,
		Message:              raw.Message,
		Provenance:           raw.Provenance,
	})
}

// verbIngest previews a directory as a ChangeSet. It is thin orchestration over
// COMMIT, not a collection framework: nothing is written until `kc writer commit`.
func verbIngest(cx *invocation) (any, error) {
	dir, err := cx.require("dir")
	if err != nil {
		return nil, err
	}
	repoID, err := cx.require("repo")
	if err != nil {
		return nil, err
	}
	repo, err := requireRepo(cx.WS, repoID)
	if err != nil {
		return nil, err
	}
	targetRef := cx.targetRef("ref")
	head, err := repo.Head(targetRef)
	if err != nil {
		return nil, err
	}
	return buildIngestPreview(cx.Flags, dir, repoID, targetRef, head)
}

// buildIngestPreview is client-safe preprocessing: it reads only the caller's
// input directory and an explicit server-derived base commit. It never opens a
// KC Home, so the typed Client can use it before sending the ChangeSet.
func buildIngestPreview(flags map[string]FlagValue, dir, repoID, targetRef string, base kernel.CommitID) (any, error) {
	preview, err := writer.Ingest(dir, kernel.RepositoryID(repoID), base)
	if err != nil {
		return nil, err
	}
	if provenance := originFrom(flags); provenance != nil {
		preview.ChangeSet.Provenance = provenance
	}
	preview.ChangeSet.TargetRef = targetRef
	if out := FlagString(flags, "out"); out != "" {
		b, err := json.MarshalIndent(preview.ChangeSet, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"changeSet":   preview.ChangeSet,
		"files":       preview.Files,
		"diagnostics": inspectIngestPreview(kernel.RepositoryID(repoID), preview),
	}, nil
}

type ingestWarning struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Address string `json:"address,omitempty"`
	Message string `json:"message"`
}

type ingestDiagnostics struct {
	Files                  int             `json:"files"`
	FrontmatterIdentities  int             `json:"frontmatterIdentities"`
	PathDerivedIdentities  int             `json:"pathDerivedIdentities"`
	KnowledgeUnits         int             `json:"knowledgeUnits"`
	SchemaObjects          int             `json:"schemaObjects"`
	ExplicitSchemaBindings int             `json:"explicitSchemaBindings"`
	SearchableBindings     int             `json:"searchableBindings"`
	UnverifiedBindings     int             `json:"unverifiedBindings"`
	Provenance             string          `json:"provenance"`
	Warnings               []ingestWarning `json:"warnings"`
}

type draftSchema struct {
	searchable bool
	err        error
}

func inspectIngestPreview(repositoryID kernel.RepositoryID, preview writer.IngestPreview) ingestDiagnostics {
	d := ingestDiagnostics{
		Files: len(preview.Files), Provenance: "SOURCE changeset envelope", Warnings: []ingestWarning{},
	}
	drafts := map[knowledge.ObjectID]draftSchema{}
	for _, op := range preview.ChangeSet.Operations {
		if !knowledge.IsSchemaObject(op.Address.ObjectID) {
			continue
		}
		d.SchemaObjects++
		desc, err := reader.InspectSchemaValue(op.Address.ObjectID, op.Value)
		drafts[op.Address.ObjectID] = draftSchema{searchable: schemaHasAccess(desc), err: err}
	}
	for _, file := range preview.Files {
		if file.IdentitySource == "frontmatter" {
			d.FrontmatterIdentities++
		} else {
			d.PathDerivedIdentities++
			d.Warnings = append(d.Warnings, ingestWarning{
				Code: "PATH_DERIVED_OBJECT_ID", Path: file.Path, Address: knowledge.AddressKey(file.Address),
				Message: "object_id comes from the relative path; add frontmatter before relying on identity across moves",
			})
		}
		if knowledge.IsSchemaObject(file.ObjectID) {
			if draft := drafts[file.ObjectID]; draft.err != nil {
				d.Warnings = append(d.Warnings, ingestWarning{
					Code: "SCHEMA_ACCESS_INVALID", Path: file.Path, Address: knowledge.AddressKey(file.Address), Message: draft.err.Error(),
				})
			}
			continue
		}
		d.KnowledgeUnits++
		if file.SchemaRef == "" {
			d.Warnings = append(d.Warnings, ingestWarning{
				Code: "SCHEMA_BINDING_UNDECLARED", Path: file.Path, Address: knowledge.AddressKey(file.Address),
				Message: "exact READ is valid, but SEARCH depends on repository-wide schema matching; add schema_ref for an explicit contract",
			})
			continue
		}
		d.ExplicitSchemaBindings++
		searchable, verified, code, err := ingestSchemaAccess(repositoryID, file.SchemaRef, drafts)
		if err != nil {
			d.Warnings = append(d.Warnings, ingestWarning{
				Code: code, Path: file.Path, Address: knowledge.AddressKey(file.Address), Message: err.Error(),
			})
			continue
		}
		if !verified {
			d.UnverifiedBindings++
			d.Warnings = append(d.Warnings, ingestWarning{
				Code: "SCHEMA_ACCESS_UNVERIFIED", Path: file.Path, Address: knowledge.AddressKey(file.Address),
				Message: "the bound schema is outside this preview; COMMIT will resolve it, then describe-schema verifies its access hints",
			})
			continue
		}
		if searchable {
			d.SearchableBindings++
			continue
		}
		d.Warnings = append(d.Warnings, ingestWarning{
			Code: "SCHEMA_HAS_NO_ACCESS_HINTS", Path: file.Path, Address: knowledge.AddressKey(file.Address),
			Message: "the bound schema declares no text/filter/sort field; exact READ works but SEARCH cannot locate this unit",
		})
	}
	return d
}

func ingestSchemaAccess(repositoryID kernel.RepositoryID, ref string, drafts map[knowledge.ObjectID]draftSchema) (searchable, verified bool, code string, err error) {
	parsed, ok := knowledge.ParseSchemaRef(ref)
	if !ok {
		return false, false, "SCHEMA_REF_UNRESOLVED", fmt.Errorf("schema_ref %q is not a schema/* reference", ref)
	}
	if parsed.Repository != "" && parsed.Repository != repositoryID {
		return false, false, "SCHEMA_REF_UNRESOLVED", fmt.Errorf("schema_ref %q names a different repository", ref)
	}
	if parsed.Commit == "" {
		if draft, exists := drafts[parsed.Object]; exists {
			if draft.err != nil {
				return false, true, "SCHEMA_ACCESS_INVALID", draft.err
			}
			return draft.searchable, true, "", nil
		}
	}
	return false, false, "", nil
}

func schemaHasAccess(schema reader.SchemaDescription) bool {
	for _, field := range schema.Fields {
		if len(field.Access) > 0 {
			return true
		}
	}
	return false
}

func verbReceipt(cx *invocation) (any, error) {
	commandID, err := cx.require("command-id")
	if err != nil {
		return nil, err
	}
	entry, ok := cx.WS.Commands.Lookup(commandID)
	if !ok {
		return nil, fmt.Errorf("unknown command-id %s", commandID)
	}
	return entry, nil
}
