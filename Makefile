GO ?= go

KC_HOME ?= /tmp/kc-demo
LISTEN ?= 127.0.0.1:7380

.PHONY: test test-component test-boundary test-e2e test-race test-cover test-plugin test-agent-e2e test-agent-ux-e2e test-data-warehouse-check test-data-warehouse test-data-warehouse-agent test-service-e2e test-state-runtime-e2e test-adapters test-docker test-all kc typecheck serve

test:
	GO=$(GO) ./scripts/testsuite.sh local

test-component:
	GO=$(GO) ./scripts/testsuite.sh component

test-boundary:
	GO=$(GO) ./scripts/testsuite.sh boundary

test-e2e:
	GO=$(GO) ./scripts/testsuite.sh e2e

test-race:
	GO=$(GO) ./scripts/testsuite.sh race

test-cover:
	GO=$(GO) ./scripts/testsuite.sh coverage

test-plugin:
	npm --prefix dsh-plugin ci --ignore-scripts --legacy-peer-deps
	npm --prefix dsh-plugin run typecheck
	npm --prefix dsh-plugin test
	npm --prefix dsh-plugin run build
	npm --prefix dsh-plugin run pack:check

# Paid, real-model acceptance. Kept explicit instead of hiding model calls in
# test/test-all; set DSH_EXECUTABLE when dsh is not on PATH.
test-agent-e2e:
	./dsh-plugin/scripts/e2e-agent-roles.sh

# Paid, real-model semantic acceptance for concept explanation, entry selection,
# and failure guidance. This does not mutate a Catalog.
test-agent-ux-e2e:
	./dsh-plugin/scripts/e2e-agent-questions.sh

# Tracked black-box provider suite. The check target is deterministic and does
# not start Docker; the other two explicitly opt into live MySQL / paid models.
test-data-warehouse-check:
	./.data/data-warehouse/check.sh

test-data-warehouse:
	./.data/data-warehouse/run.sh

test-data-warehouse-agent:
	./.data/data-warehouse/run-agent.sh

test-service-e2e:
	GO=$(GO) ./scripts/testsuite.sh service-e2e

test-state-runtime-e2e:
	GO=$(GO) ./scripts/testsuite.sh state-runtime

test-adapters:
	GO=$(GO) ./scripts/testsuite.sh adapters

test-docker:
	GO=$(GO) ./scripts/testsuite.sh docker

test-all:
	GO=$(GO) ./scripts/testsuite.sh all
	$(MAKE) test-plugin

kc:
	$(GO) run ./cmd/kc -- $(ARGS)

serve:
	$(GO) run ./cmd/kc -- serve --home $(KC_HOME) --listen $(LISTEN)

typecheck:
	$(GO) test -run '^$$' ./...
	npm --prefix dsh-plugin run typecheck
