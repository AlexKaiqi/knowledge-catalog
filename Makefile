GO ?= go

KC_HOME ?= /tmp/kc-demo
LISTEN ?= 127.0.0.1:7380

.PHONY: test kc typecheck serve

test:
	$(GO) test ./...

kc:
	$(GO) run ./cmd/kc -- $(ARGS)

serve:
	$(GO) run ./cmd/kc -- serve --home $(KC_HOME) --listen $(LISTEN)

typecheck:
	$(GO) test -c -o /dev/null ./...
