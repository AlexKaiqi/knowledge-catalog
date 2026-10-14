package format

import (
	"encoding/json"
	"path"
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

const LocatorSchemaReferrersDirectory = ".kc/object-locators/schema-referrers"
const LocatorReferrersCompletePath = ".kc/object-locators/schema-referrers.complete"
const LocatorReferrersCompleteBody = "v1\n"

type schemaReferrersEntry struct {
	Schema    objectmodel.ObjectID  `json:"schema"`
	Addresses []objectmodel.Address `json:"addresses"`
}

func SchemaReferrersPath(schema objectmodel.ObjectID) string {
	name := kernel.CanonicalDigest(map[string]any{"schema": schema})
	return path.Join(LocatorSchemaReferrersDirectory, string(name)+".entry")
}

func EncodeSchemaReferrers(schema objectmodel.ObjectID, addresses []objectmodel.Address) ([]byte, error) {
	entry := schemaReferrersEntry{Schema: schema, Addresses: append([]objectmodel.Address(nil), addresses...)}
	if err := validateSchemaReferrers(entry, schema); err != nil {
		return nil, err
	}
	sort.Slice(entry.Addresses, func(i, j int) bool {
		return objectmodel.AddressKey(entry.Addresses[i]) < objectmodel.AddressKey(entry.Addresses[j])
	})
	return json.Marshal(entry)
}

// ReadSchemaReferrers uses only fixed metadata. A complete directory proves an
// absent entry has no referrers; older authorities require explicit rebuild.
func ReadSchemaReferrers(tree snapshot.TreeReader, schema objectmodel.ObjectID, commit kernel.CommitID) ([]objectmodel.Address, error) {
	if !objectmodel.IsSchemaObject(schema) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "Schema referrer lookup requires a schema/* identity")
	}
	raw, err := tree.ReadFile(SchemaReferrersPath(schema), commit)
	if err == nil {
		return decodeSchemaReferrers(raw, schema)
	}
	if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return nil, err
	}
	return readMissingSchemaReferrers(tree, schema, commit)
}

func decodeSchemaReferrers(raw []byte, schema objectmodel.ObjectID) ([]objectmodel.Address, error) {
	var entry schemaReferrersEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid Schema referrer coordinates: %v", err)
	}
	if err := validateSchemaReferrers(entry, schema); err != nil {
		return nil, err
	}
	sort.Slice(entry.Addresses, func(i, j int) bool {
		return objectmodel.AddressKey(entry.Addresses[i]) < objectmodel.AddressKey(entry.Addresses[j])
	})
	return entry.Addresses, nil
}

func readMissingSchemaReferrers(tree snapshot.TreeReader, schema objectmodel.ObjectID, commit kernel.CommitID) ([]objectmodel.Address, error) {
	raw, err := tree.ReadFile(LocatorReferrersCompletePath, commit)
	if err == nil {
		if string(raw) == LocatorReferrersCompleteBody {
			return nil, nil
		}
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid Schema referrer completeness marker at %s", commit)
	}
	if err != nil && kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return nil, err
	}
	raw, err = tree.ReadFile(LocatorManifestPath, commit)
	if err == nil {
		manifest, err := DecodeLocatorManifest(raw)
		if err != nil {
			return nil, err
		}
		if manifest.Referrers != nil {
			entry := schemaReferrersEntry{Schema: schema, Addresses: manifest.Referrers[schema]}
			encoded, err := json.Marshal(entry)
			if err != nil {
				return nil, err
			}
			return decodeSchemaReferrers(encoded, schema)
		}
	} else if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return nil, err
	}
	return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
		"object locator lacks complete Schema referrers at %s; explicitly rebuild locators", commit)
}

func validateSchemaReferrers(entry schemaReferrersEntry, expected objectmodel.ObjectID) error {
	if entry.Schema != expected || !objectmodel.IsSchemaObject(entry.Schema) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "Schema referrer locator has invalid identity %s", entry.Schema)
	}
	seen := map[string]bool{}
	for _, address := range entry.Addresses {
		key := objectmodel.AddressKey(address)
		if objectmodel.IsSchemaObject(address.ObjectID) || objectmodel.AssertWritable(address) != nil || seen[key] {
			return kernel.Fail(kernel.ErrPreconditionFailed, "Schema referrer locator has invalid or duplicate Address %s", key)
		}
		seen[key] = true
	}
	return nil
}
