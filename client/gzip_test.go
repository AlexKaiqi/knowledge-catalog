package client_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/client"
	"kc/knowledge"
)

// TestTypedClientGzipsLargeRequestBodies pins the transport-compression
// contract: marshaled request bodies above the 512 KiB threshold travel as
// Content-Encoding: gzip, so a scale ChangeSet stops paying the raw-JSON wire
// cost. The server decode side is covered by the facade tests.
func TestTypedClientGzipsLargeRequestBodies(t *testing.T) {
	var gotEncoding string
	var decoded []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if gotEncoding == "gzip" {
			gz, err := gzipReader(body)
			if err != nil {
				t.Errorf("gunzip: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body = gz
		}
		decoded = body
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "APPLIED"})
	}))
	defer server.Close()
	kc, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("k", 600<<10)
	receipt := map[string]any{}
	err = kc.WriterService().Commit(context.Background(), "kr://kaiqidong/compression",
		client.CommitRequest{CommandID: "gzip-1", ChangeSet: knowledge.ChangeSet{
			TargetRepository: "kr://kaiqidong/compression",
			Operations: []knowledge.Operation{{
				Op:      knowledge.OpPut,
				Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "obj/big"},
				Value:   map[string]any{"body": big},
			}},
		}}, client.RequestOptions{}, &receipt)
	if err != nil {
		t.Fatalf("gzip commit failed: %v", err)
	}
	if gotEncoding != "gzip" {
		t.Fatalf("large request body traveled as Content-Encoding %q", gotEncoding)
	}
	var sent struct {
		ChangeSet knowledge.ChangeSet `json:"changeSet"`
	}
	if err := json.Unmarshal(decoded, &sent); err != nil {
		t.Fatalf("decoded body is not the original JSON: %v", err)
	}
	value, _ := sent.ChangeSet.Operations[0].Value.(map[string]any)
	if value["body"] != big {
		t.Fatalf("decoded payload lost bytes: %d", len(decoded))
	}
}

// TestTypedClientSmallRequestBodiesStayUncompressed keeps small calls plain so
// the compression never adds latency where it cannot pay for itself.
func TestTypedClientSmallRequestBodiesStayUncompressed(t *testing.T) {
	var gotEncoding string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding = r.Header.Get("Content-Encoding")
		_, _ = io.Copy(io.Discard, r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()
	kc, err := client.New(client.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "kaiqidong"}}); err != nil {
		t.Fatal(err)
	}
	if err := kc.WriterService().Head(context.Background(), "kr://kaiqidong/small", "", client.RequestOptions{}, &map[string]any{}); err != nil {
		t.Fatalf("small request failed: %v", err)
	}
	if gotEncoding != "" {
		t.Fatalf("small request body sent Content-Encoding %q", gotEncoding)
	}
}

func gzipReader(body []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(gz)
}
