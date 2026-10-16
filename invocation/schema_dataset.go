package invocation

import (
	"encoding/base64"
	"encoding/json"
	"sort"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// Search readiness of one Dataset member as seen by a consumer describe.
const (
	SchemaSearchReady       = "ready"
	SchemaSearchUnavailable = "unavailable"
)

// DatasetSchemaReport is one member's Schema report inside a Dataset revision.
// Field access is filled from the index declarations selected into the same
// Dataset, so a consumer learns structure and searchable uses in one call.
type DatasetSchemaReport struct {
	reader.SchemaReport
	Dataset  string       `json:"dataset"`
	Revision int          `json:"revision"`
	Search   SchemaSearch `json:"search"`
}

// SchemaSearch states whether this member can serve search at this revision
// and which declaration members contribute to (or fail for) its Schemas.
type SchemaSearch struct {
	State        string                        `json:"state"`
	Reason       string                        `json:"reason,omitempty"`
	Declarations []retrieval.DeclarationStatus `json:"declarations"`
}

// DatasetSchemaEntry is one row of `kc schema list --dataset`.
type DatasetSchemaEntry struct {
	Repository  kernel.RepositoryID  `json:"repository"`
	Commit      kernel.CommitID      `json:"commit"`
	ObjectID    objectmodel.ObjectID `json:"objectId"`
	Entity      string               `json:"entity,omitempty"`
	Aspect      string               `json:"aspect,omitempty"`
	Description string               `json:"description,omitempty"`
	Fields      int                  `json:"fields"`
	Uses        []reader.AccessHint  `json:"uses"`
}

// DatasetSchemaPage is one page of Schemas delivered by a Dataset revision.
type DatasetSchemaPage struct {
	Dataset      string               `json:"dataset"`
	Revision     int                  `json:"revision"`
	Schemas      []DatasetSchemaEntry `json:"schemas"`
	Continuation string               `json:"continuation,omitempty"`
}

// describeDatasetSchemas annotates Dataset-scoped Schema reports with the
// faces the Dataset's own declarations give each field. Declarations outside
// the Dataset do not appear: the scoped member cannot read them.
func describeDatasetSchemas(cx *Invocation, serving *reader.Serving, objectID objectmodel.ObjectID) ([]DatasetSchemaReport, error) {
	reports, err := serving.DescribeSchema(objectID)
	if err != nil {
		return nil, err
	}
	pin := serving.Pin()
	out := []DatasetSchemaReport{}
	for _, report := range reports {
		if !AllowedRepoRead(cx.Home, cx.Flags, string(report.Repository), string(objectID)) {
			continue
		}
		member, err := serving.Member(report.Repository)
		if err != nil {
			return nil, err
		}
		if cx.WS.Plugins != nil && cx.WS.Plugins.Configured() {
			member, err = cx.WS.Plugins.DatasetMember(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
				return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
			}, pin, report.Repository)
			if err != nil {
				return nil, err
			}
		}
		described := DatasetSchemaReport{SchemaReport: report, Dataset: pin.SetID, Revision: pin.Revision,
			Search: SchemaSearch{State: SchemaSearchUnavailable, Declarations: []retrieval.DeclarationStatus{}}}
		if cx.WS.Index == nil {
			described.Search.Reason = "no index plugin is installed; declarations are stored but not interpreted"
			out = append(out, described)
			continue
		}
		spec, err := cx.WS.Index.AccessSpecAt(member, report.Commit)
		if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			described.Search.Reason = err.Error()
			out = append(out, described)
			continue
		}
		if err != nil {
			return nil, err
		}
		described.SchemaReport = annotateReport(report, spec)
		described.Search.Declarations = declarationsFor(report, spec, objectID == "")
		described.Search.State, described.Search.Reason = memberReadiness(cx, member, report.Commit)
		out = append(out, described)
	}
	return out, nil
}

// memberReadiness asks the installed index whether this member's fixed commit
// can be searched now. Not ready is reported, never shown as an empty index.
func memberReadiness(cx *Invocation, member objectmodel.Repository, commit kernel.CommitID) (string, string) {
	if _, err := cx.WS.Index.CheckSearchProjectionAt(member, commit); err != nil {
		return SchemaSearchUnavailable, err.Error()
	}
	return SchemaSearchReady, ""
}

func annotateReport(report reader.SchemaReport, spec retrieval.AccessSpec) reader.SchemaReport {
	faces := map[string][]reader.AccessHint{}
	for _, field := range spec.Fields {
		key := string(field.Schema) + "\x1f" + field.Path
		faces[key] = append(faces[key], field.Access...)
	}
	out := report
	out.Schemas = make([]reader.SchemaDescription, len(report.Schemas))
	for i, schema := range report.Schemas {
		copied := schema
		copied.Fields = make([]reader.FieldAccess, len(schema.Fields))
		for j, field := range schema.Fields {
			field.Access = uniqueHints(faces[string(schema.ObjectID)+"\x1f"+field.Path])
			copied.Fields[j] = field
		}
		out.Schemas[i] = copied
	}
	return out
}

// declarationsFor keeps the members whose inputs name a Schema in this report.
// A member that failed before resolving inputs is shown only when the
// describe covers every Schema, since it cannot be attributed to one.
func declarationsFor(report reader.SchemaReport, spec retrieval.AccessSpec, all bool) []retrieval.DeclarationStatus {
	schemas := map[objectmodel.ObjectID]bool{}
	for _, schema := range report.Schemas {
		schemas[schema.ObjectID] = true
	}
	out := []retrieval.DeclarationStatus{}
	for _, status := range spec.Statuses() {
		keep := all && len(status.Inputs) == 0
		for _, input := range status.Inputs {
			keep = keep || schemas[input.Schema]
		}
		if keep {
			out = append(out, status)
		}
	}
	return out
}

func uniqueHints(hints []reader.AccessHint) []reader.AccessHint {
	seen := map[reader.AccessHint]bool{}
	out := []reader.AccessHint{}
	for _, hint := range hints {
		if !seen[hint] {
			seen[hint] = true
			out = append(out, hint)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

type schemaPageCursor struct {
	Pin    kernel.Digest `json:"pin"`
	Offset int           `json:"offset"`
}

// browseDatasetSchemas lists the Schemas a Dataset revision delivers. The
// continuation is bound to the frozen pin; a page from another revision is
// rejected rather than silently mixed.
func browseDatasetSchemas(cx *Invocation, serving *reader.Serving, limit int) (DatasetSchemaPage, error) {
	reports, err := describeDatasetSchemas(cx, serving, "")
	if err != nil {
		return DatasetSchemaPage{}, err
	}
	pin := serving.Pin()
	digest := kernel.CanonicalDigest(pin)
	offset := 0
	if raw := cx.Flag("continuation"); raw != "" {
		var cursor schemaPageCursor
		body, decodeErr := base64.RawURLEncoding.DecodeString(raw)
		if decodeErr != nil || json.Unmarshal(body, &cursor) != nil || cursor.Offset < 0 {
			return DatasetSchemaPage{}, kernel.Fail(kernel.ErrUsageInvalid, "--continuation is not a schema list cursor")
		}
		if cursor.Pin != digest {
			return DatasetSchemaPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "--continuation belongs to another Dataset revision; pass the --revision of the first page")
		}
		offset = cursor.Offset
	}
	entries := []DatasetSchemaEntry{}
	for _, report := range reports {
		for _, schema := range report.Schemas {
			uses := []reader.AccessHint{}
			for _, field := range schema.Fields {
				uses = append(uses, field.Access...)
			}
			entries = append(entries, DatasetSchemaEntry{Repository: report.Repository, Commit: report.Commit, ObjectID: schema.ObjectID,
				Entity: schema.Entity, Aspect: schema.Aspect, Description: schema.Definition.Description,
				Fields: len(schema.Fields), Uses: uniqueHints(uses)})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].ObjectID != entries[j].ObjectID {
			return entries[i].ObjectID < entries[j].ObjectID
		}
		return entries[i].Repository < entries[j].Repository
	})
	page := DatasetSchemaPage{Dataset: pin.SetID, Revision: pin.Revision, Schemas: []DatasetSchemaEntry{}}
	if offset > len(entries) {
		offset = len(entries)
	}
	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}
	page.Schemas = append(page.Schemas, entries[offset:end]...)
	if end < len(entries) {
		body, _ := json.Marshal(schemaPageCursor{Pin: digest, Offset: end})
		page.Continuation = base64.RawURLEncoding.EncodeToString(body)
	}
	return page, nil
}
