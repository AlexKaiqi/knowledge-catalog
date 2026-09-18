GO ?= go

KC_DEPLOYMENT_CONFIG ?= /tmp/kc-demo/deployment.json
LISTEN ?= 127.0.0.1:7380

DOCS_LISTEN ?= 127.0.0.1:8766

.PHONY: check-docs check-surface quality test test-component test-boundary test-e2e test-race test-cover test-plugin test-agent-e2e test-agent-metric-e2e test-agent-ux-e2e test-service-e2e test-taihu-live test-state-runtime-e2e test-kcfs-e2e test-adapters test-docker test-all system-gitea-up system-gitea-status system-gitea-down system-lakefs-up system-lakefs-status system-lakefs-down deploy-local-up deploy-local-status deploy-local-access deploy-local-smoke deploy-local-scenes deploy-local-goto deploy-local-down deploy-dev-up deploy-dev-status deploy-dev-access deploy-dev-smoke deploy-dev-down deploy-dev-reset kc typecheck serve docs-serve

check-docs:
	$(GO) run ./scripts/check-docs

.PHONY: validation-inventory check-validation
validation-inventory:
	@GO=$(GO) python3 ./scripts/validation.py inventory

check-validation:
	python3 ./scripts/validation_test.py
	$(GO) test -json -count=1 ./scripts/validation-inventory
	$(GO) run ./scripts/validation-inventory --check

.PHONY: check-observability
check-observability:
	./scripts/check-observability.sh

docs-serve:
	$(GO) run ./scripts/docs-serve --listen $(DOCS_LISTEN)

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
	$(MAKE) check-surface
	GO=$(GO) ./scripts/testsuite.sh coverage

test-plugin:
	node --test cli/web/repository.test.mjs cli/web/console.test.mjs
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

test-service-e2e:
	GO=$(GO) ./scripts/testsuite.sh service-e2e

# Real Taihu introspection. Skips unless KC_LIVE_TAIHU=1 and the env secrets
# are set; the browser login helper is scripts/live-taihu-auth.sh.
test-taihu-live:
	GO=$(GO) ./scripts/testsuite.sh taihu-live

test-state-runtime-e2e:
	GO=$(GO) ./scripts/testsuite.sh state-runtime

# Real Linux FUSE acceptance. On macOS this runs inside Docker with /dev/fuse
# and SYS_ADMIN, including the DSH MountController -> kcfs daemon lifecycle.
test-kcfs-e2e:
	GO=$(GO) ./scripts/testsuite.sh kcfs

test-adapters:
	GO=$(GO) ./scripts/testsuite.sh adapters

test-docker:
	GO=$(GO) ./scripts/testsuite.sh docker

test-all:
	$(MAKE) check-surface
	GO=$(GO) ./scripts/testsuite.sh all
	$(MAKE) test-plugin

system-gitea-up:
	./scripts/system-gitea.sh up

system-gitea-status:
	./scripts/system-gitea.sh status

system-gitea-down:
	./scripts/system-gitea.sh down

system-lakefs-up:
	./scripts/system-lakefs.sh up

system-lakefs-status:
	./scripts/system-lakefs.sh status

system-lakefs-down:
	./scripts/system-lakefs.sh down

deploy-local-up:
	./scripts/deploy/deploy.sh local up

deploy-local-status:
	./scripts/deploy/deploy.sh local status

deploy-local-access:
	./scripts/deploy/deploy.sh local access

deploy-local-smoke:
	./scripts/deploy/deploy.sh local smoke

deploy-local-scenes:
	./scripts/deploy/deploy.sh local scenes

deploy-local-goto:
	./scripts/deploy/deploy.sh local goto $(NODE)

deploy-local-down:
	./scripts/deploy/deploy.sh local down

deploy-dev-up:
	./scripts/deploy/deploy.sh dev up

deploy-dev-status:
	./scripts/deploy/deploy.sh dev status

deploy-dev-access:
	./scripts/deploy/deploy.sh dev access

deploy-dev-smoke:
	./scripts/deploy/deploy.sh dev smoke

deploy-dev-down:
	./scripts/deploy/deploy.sh dev down

deploy-dev-reset:
	./scripts/deploy/deploy.sh dev reset

kc:
	$(GO) run ./cmd/kc -- $(ARGS)

serve:
	$(GO) run ./cmd/kc -- serve --config $(KC_DEPLOYMENT_CONFIG) --listen $(LISTEN)

typecheck:
	$(GO) test -run '^$$' ./...
	npm --prefix dsh-plugin run typecheck
