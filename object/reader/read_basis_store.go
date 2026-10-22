package reader

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

var readBasisBucket = []byte("object-read-bases-v1")

// ReadBasisStore retains service-issued coordinates independently of search
// generations. Retain returns only after durable storage; Resolve never creates
// a record or substitutes current coordinates for a missing record. Records
// contain no permission, observation or source content.
type ReadBasisStore interface {
	Retain(context.Context, objectmodel.ReadBasis) (string, error)
	Resolve(context.Context, string) (objectmodel.ReadBasis, error)
}

// NewReadBasisStore binds metadata storage to a host-owned durable state file.
// It owns no background work or open handle. Metadata is deduplicated by exact
// fixed basis and retained for the installation's lifetime; source availability
// and current access are checked separately when reopening the basis.
func NewReadBasisStore(path string) (ReadBasisStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "read basis storage requires a durable state path")
	}
	return &readBasisStore{path: path}, nil
}

type readBasisStore struct{ path string }

func (s *readBasisStore) Retain(ctx context.Context, basis objectmodel.ReadBasis) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := basis.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(basis)
	if err != nil {
		return "", err
	}
	reference := readBasisReference(basis)
	err = (coordinateRecords{path: s.path, bucket: readBasisBucket}).put(ctx, reference, encoded, func(raw []byte) error { _, err := decodeReadBasisRecord(raw, reference); return err })
	if err != nil {
		return "", err
	}

	return reference, nil
}

func (s *readBasisStore) Resolve(ctx context.Context, reference string) (objectmodel.ReadBasis, error) {
	if err := ctx.Err(); err != nil {
		return objectmodel.ReadBasis{}, err
	}
	if !validReadBasisReference(reference) {
		return objectmodel.ReadBasis{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid read basis reference")
	}
	raw, err := (coordinateRecords{path: s.path, bucket: readBasisBucket}).get(ctx, reference)
	if err != nil {
		return objectmodel.ReadBasis{}, err
	}
	return decodeReadBasisRecord(raw, reference)
}

func readBasisReference(basis objectmodel.ReadBasis) string {
	return "rb1_" + string(kernel.CanonicalDigest(basis))
}

func validReadBasisReference(reference string) bool {
	if !strings.HasPrefix(reference, "rb1_") || len(reference) != 68 {
		return false
	}
	_, err := hex.DecodeString(reference[4:])
	return err == nil
}

func decodeReadBasisRecord(raw []byte, reference string) (objectmodel.ReadBasis, error) {
	var basis objectmodel.ReadBasis
	if err := json.Unmarshal(raw, &basis); err != nil || basis.Validate() != nil || readBasisReference(basis) != reference {
		return objectmodel.ReadBasis{}, kernel.Fail(kernel.ErrPreconditionFailed, "durable read basis record is corrupt")
	}
	return basis, nil
}
