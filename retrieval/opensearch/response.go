package opensearch

import (
	"bytes"
	"encoding/json"
	"io"

	"kc/kernel"
	"kc/knowledge"
)

type shardResponse struct {
	Total      int `json:"total"`
	Successful int `json:"successful"`
	Failed     int `json:"failed"`
}

func (s *shardResponse) check(operation string) error {
	if s == nil || s.Total <= 0 || s.Successful != s.Total || s.Failed != 0 {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch %s did not complete all shards", operation)
	}
	return nil
}

// Decoding interface values as float64 changes long sort boundaries above 2^53.
// Keep the original JSON numbers through response, continuation, and search_after.
func decodeJSON(body []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid opensearch response: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid opensearch response: trailing JSON content")
	}
	return nil
}

func decodeSearchResponse(body []byte, sortArity int) ([]knowledge.ObjectID, [][]any, string, error) {
	var response struct {
		PIT             string         `json:"pit_id"`
		TimedOut        *bool          `json:"timed_out"`
		TerminatedEarly bool           `json:"terminated_early"`
		Shards          *shardResponse `json:"_shards"`
		Hits            *struct {
			Hits []struct {
				Source struct {
					ObjectID string `json:"object_id"`
				} `json:"_source"`
				Sort []any `json:"sort"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := decodeJSON(body, &response); err != nil {
		return nil, nil, "", err
	}
	if err := response.Shards.check("search"); err != nil {
		return nil, nil, "", err
	}
	if response.TimedOut == nil || *response.TimedOut || response.TerminatedEarly || response.Hits == nil || response.Hits.Hits == nil {
		return nil, nil, "", kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch search did not return a complete result")
	}
	ids := make([]knowledge.ObjectID, 0, len(response.Hits.Hits))
	sorts := make([][]any, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		if hit.Source.ObjectID == "" || len(hit.Sort) < sortArity {
			return nil, nil, "", kernel.Fail(kernel.ErrTemporaryUnavailable, "opensearch search returned a candidate without identity or sort boundary")
		}
		ids = append(ids, knowledge.ObjectID(hit.Source.ObjectID))
		sorts = append(sorts, hit.Sort)
	}
	return ids, sorts, response.PIT, nil
}
