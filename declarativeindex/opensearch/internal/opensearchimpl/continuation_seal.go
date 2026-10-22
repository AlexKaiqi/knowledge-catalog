package opensearchimpl

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	ports "kc/discovery/projection"
	"kc/kernel"
)

const cursorKeyState = "CONTINUATION_KEY_V1"

type continuationSealer struct {
	key    []byte
	domain string
}

func (s *continuationSealer) signature(body []byte) []byte {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(s.domain))
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}
func (s *continuationSealer) SealContinuation(body []byte) string {
	signed := make([]byte, 0, len(body)+sha256.Size)
	signed = append(signed, body...)
	signed = append(signed, s.signature(body)...)
	return base64.RawURLEncoding.EncodeToString(signed)
}
func (s *continuationSealer) OpenContinuation(token string) ([]byte, error) {
	signed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(signed) <= sha256.Size {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid authenticated continuation")
	}
	body, signature := signed[:len(signed)-sha256.Size], signed[len(signed)-sha256.Size:]
	if !hmac.Equal(signature, s.signature(body)) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation authentication failed")
	}
	return body, nil
}

func (e *openSearchEngine) LoadContinuationSealer(ctx context.Context) (ports.ContinuationSealer, error) {
	record, version, err := e.loadControlRecord(ctx, e.controlID+"-cursor-key")
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation signing material is unavailable")
	}
	return e.sealerFromRecord(record)
}

func (e *openSearchEngine) sealerFromRecord(record controlDoc) (ports.ContinuationSealer, error) {
	if record.State != cursorKeyState || record.Repository != string(e.repository) || len(record.CursorKey) != sha256.Size || record.ActiveIndex != "" || record.Generation != "" || record.Resource != nil {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation signing material has unknown identity or format")
	}
	return &continuationSealer{key: append([]byte(nil), record.CursorKey...), domain: e.controlIndexPath() + "\x00" + string(e.repository) + "\x00"}, nil
}

func (e *openSearchEngine) PrepareContinuationSealer(ctx context.Context) (ports.ContinuationSealer, error) {
	record, version, err := e.loadControlRecord(ctx, e.controlID+"-cursor-key")
	if err != nil {
		return nil, err
	}
	if version != nil {
		return e.sealerFromRecord(record)
	}
	if err := e.ensureControlIndexContext(ctx); err != nil {
		return nil, err
	}
	record = controlDoc{State: cursorKeyState, Repository: string(e.repository), CursorKey: make([]byte, sha256.Size)}
	if _, err := rand.Read(record.CursorKey); err != nil {
		return nil, err
	}
	if _, err := e.putControlRecordContext(ctx, e.controlID+"-cursor-key", record, nil); err != nil {
		// An accepted-but-lost create or another creator may own the immutable key.
		// No token has used our candidate yet; only actual durable material is valid.
		adopted, readErr := e.LoadContinuationSealer(ctx)
		if readErr != nil {
			return nil, errors.Join(err, readErr)
		}
		return adopted, nil
	}
	return e.sealerFromRecord(record)
}
