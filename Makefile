GO ?= go

KC_HOME ?= /tmp/kc-demo
LISTEN ?= 127.0.0.1:7380

.PHONY: test test-component test-boundary test-e2e test-adapters test-docker test-all kc typecheck serve

test:
	GO=$(GO) ./scripts/testsuite.sh local

test-component:
	GO=$(GO) ./scripts/testsuite.sh component

test-boundary:
	GO=$(GO) ./scripts/testsuite.sh boundary

test-e2e:
	GO=$(GO) ./scripts/testsuite.sh e2e

test-adapters:
	GO=$(GO) ./scripts/testsuite.sh adapters

test-docker:
	GO=$(GO) ./scripts/testsuite.sh docker

test-all:
	GO=$(GO) ./scripts/testsuite.sh all

kc:
	$(GO) run ./cmd/kc -- $(ARGS)

serve:
	$(GO) run ./cmd/kc -- serve --home $(KC_HOME) --listen $(LISTEN)

typecheck:
	$(GO) test -run '^$$' ./...
