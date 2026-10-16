package opensearchimpl

import (
	"testing"

	"kc/kernel"
)

func TestPublishedEmbeddingModelUsesTheEngineConfigurationValidator(t *testing.T) {
	for _, dims := range []string{"", "not-a-number", "0", "16385", "64"} {
		t.Run(dims, func(t *testing.T) {
			t.Setenv(EnvEmbeddingModel, "embed/test-hash@1")
			t.Setenv(EnvEmbeddingDims, dims)
			models, err := ConfiguredEmbeddingModels()
			if dims == "64" {
				if err != nil || len(models) != 1 || models[0] != "embed/test-hash@1" {
					t.Fatalf("configured model was not published: %#v %v", models, err)
				}
			} else if kernel.CodeOf(err) != kernel.ErrUsageInvalid || len(models) != 0 {
				t.Fatalf("model without usable dimensions was published: %#v %v", models, err)
			}
		})
	}
}
