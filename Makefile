GO ?= go

KC_HOME ?= /tmp/kc-demo
LISTEN ?= 127.0.0.1:7380

.PHONY: check-docs check-surface quality test test-component test-boundary test-e2e test-race test-cover test-plugin test-agent-e2e test-agent-metric-e2e test-agent-ux-e2e test-data-warehouse-check test-data-warehouse test-data-warehouse-agent test-service-e2e test-taihu-live test-state-runtime-e2e test-kcfs-e2e test-adapters test-docker test-all dw-env-up dw-env-smoke dw-env-status dw-env-down dw-env-reset dw-obs-up dw-obs-smoke dw-obs-down system-gitea-up system-gitea-status system-gitea-down kc typecheck serve

check-docs:
	$(GO) run ./scripts/check-docs

check-surface:
	$(MAKE) check-docs
	./scripts/check-surface.sh

quality:
	$(MAKE) check-docs
	GO=$(GO) ./scripts/check-quality.sh

test:
	$(MAKE) check-surface
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

# Paid companion: metric permission briefs from the protocol feature file.
# Go Then remains the oracle; this only checks Agent trace and answer markers.
test-agent-metric-e2e:
	./dsh-plugin/scripts/e2e-agent-metric-permission.sh

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

# Reproducible macOS/Linux development topology. Default Client is HTTP bash
# (ttyd + kc). Optional DSH + kcfs use the Linux client container; the KC
# Server composes one Dolt and one Gitea Repository.
dw-env-up:
	./.data/data-warehouse/dev.sh up

dw-env-smoke:
	./.data/data-warehouse/dev.sh smoke

dw-env-status:
	./.data/data-warehouse/dev.sh status

dw-env-down:
	./.data/data-warehouse/dev.sh down

dw-env-reset:
	./.data/data-warehouse/dev.sh reset

# Optional local observability profile around the same real KC Server workload.
dw-obs-up:
	./.data/data-warehouse/dev.sh obs-up

dw-obs-smoke:
	./.data/data-warehouse/dev.sh obs-smoke

dw-obs-down:
	./.data/data-warehouse/dev.sh obs-down

test-service-e2e:
	GO=$(GO) ./scripts/testsuite.sh service-e2e

# Real Taihu introspection. Skips unless KC_LIVE_TAIHU=1 and the env secrets
# are set; the browser login helper is scripts/live-taihu-auth.sh.
test-taihu-live:
	KC_LIVE_TAIHU=1 $(GO) test -count=1 -timeout=2m -run TestLiveTaihuAuthentication ./cli

test-state-runtime-e2e:
	GO=$(GO) ./scripts/testsuite.sh state-runtime

# Real Linux FUSE acceptance. On macOS this runs inside Docker with /dev/fuse
# and SYS_ADMIN, including the DSH MountController -> kcfs daemon lifecycle.
test-kcfs-e2e:
	./scripts/e2e-kcfs-docker.sh

test-adapters:
	GO=$(GO) ./scripts/testsuite.sh adapters

test-docker:
	GO=$(GO) ./scripts/testsuite.sh docker

test-all:
	GO=$(GO) ./scripts/testsuite.sh all
	$(MAKE) test-plugin

system-gitea-up:
	./scripts/system-gitea.sh up

system-gitea-status:
	./scripts/system-gitea.sh status

system-gitea-down:
	./scripts/system-gitea.sh down

kc:
	$(GO) run ./cmd/kc -- $(ARGS)

serve:
	$(GO) run ./cmd/kc -- serve --home $(KC_HOME) --listen $(LISTEN)

typecheck:
	$(GO) test -run '^$$' ./...
	npm --prefix dsh-plugin run typecheck
