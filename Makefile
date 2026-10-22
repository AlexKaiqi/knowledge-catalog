GO ?= go
PYTHON ?= .venv/bin/python
VERSION ?= 0.0.0
DIST_DIR ?= dist
TARGET ?=

KC_DEPLOYMENT_CONFIG ?= /tmp/kc-demo/deployment.json
LISTEN ?= 127.0.0.1:7380

DOCS_LISTEN ?= 127.0.0.1:8766

.PHONY: check-docs check-surface quality env-doctor test test-component test-boundary test-e2e test-race test-cover test-taihu-live test-state-runtime-e2e test-kcfs-e2e test-adapters test-docker test-all system-lakefs-up system-lakefs-status system-lakefs-down deploy-local-up deploy-local-status deploy-local-access deploy-local-smoke deploy-local-scenes deploy-local-goto deploy-local-accept deploy-local-down deploy-dev-up deploy-dev-status deploy-dev-access deploy-dev-smoke deploy-dev-down deploy-dev-reset kc typecheck serve docs-serve

# Read-only development environment report. Never starts, stops, or removes anything.
env-doctor:
	./scripts/env-doctor.sh
.PHONY: test-lakefs test-contracts
.PHONY: quality-webui test-webui
.PHONY: build package package-all test-package

quality-webui:
	npm --prefix snapshot/console/webui run typecheck
	npm --prefix snapshot/console/webui run lint:console

test-webui: quality-webui
	npm --prefix snapshot/console/webui run build
	npm --prefix snapshot/console/webui test
	$(GO) test ./snapshot/console ./cli ./server ./invocation -run 'TestDataset(Console|Management|UI)' -count=1

check-docs:
	$(GO) run ./scripts/check-docs

.PHONY: validation-inventory validation-prune leases-prune check-validation
validation-inventory:
	@GO=$(GO) python3 ./scripts/validation.py inventory

# Dry run by default; pass APPLY=--apply to delete. Runs cited by a tracked file are never pruned.
validation-prune:
	@python3 ./scripts/validation.py prune --keep $(or $(KEEP),20) $(if $(SCOPE),--scope $(SCOPE)) $(APPLY)

# Dry run by default; pass APPLY=--apply to delete. A lease is removed only when
# its Docker daemon is gone, no process holds its lock, and its container is absent.
leases-prune:
	@python3 ./scripts/opensearch_auto.py leases $(APPLY)

check-validation:
	python3 ./scripts/validation_test.py
	python3 ./scripts/maintenance_test.py
	$(MAKE) test-package
	$(GO) test -json -count=1 ./scripts/check-docs ./scripts/validation-inventory
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

# Default scene fixtures; real lakeFS deployment remains deploy-local-scenes.
test:
	$(MAKE) check-surface
	GO=$(GO) ./scripts/testsuite.sh lakefs

test-lakefs: test

# Explicit older mixed-provider contracts; not the product deployment default.
test-contracts:
	$(MAKE) check-surface
	GO=$(GO) ./scripts/testsuite.sh contracts

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

# Real Taihu introspection. Skips unless KC_LIVE_TAIHU=1 and the env secrets
# are set; the browser login helper is scripts/live-taihu-auth.sh.
test-taihu-live:
	GO=$(GO) ./scripts/testsuite.sh taihu-live

test-state-runtime-e2e:
	GO=$(GO) ./scripts/testsuite.sh state-runtime

# Real Linux FUSE acceptance. On macOS this runs inside Docker with /dev/fuse
# and SYS_ADMIN, including the native kcfs daemon lifecycle.
test-kcfs-e2e:
	GO=$(GO) ./scripts/testsuite.sh kcfs

test-adapters:
	GO=$(GO) ./scripts/testsuite.sh adapters

test-docker:
	GO=$(GO) ./scripts/testsuite.sh docker

test-all:
	$(MAKE) check-surface
	GO=$(GO) ./scripts/testsuite.sh all


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

deploy-local-accept:
	./scripts/deploy/deploy.sh local accept $(if $(SPEC),--select $(SPEC))

deploy-local-scenes:
	./scripts/deploy/deploy.sh local scenes

deploy-local-goto:
	./scripts/deploy/deploy.sh local goto $(NODE) $(if $(PROBE),--probe $(PROBE))

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

build:
	GO="$(GO)" $(PYTHON) ./scripts/package/build.py build --version "$(VERSION)" --output "$(DIST_DIR)" $(if $(TARGET),--target "$(TARGET)")

package:
	GO="$(GO)" $(PYTHON) ./scripts/package/build.py package --version "$(VERSION)" --output "$(DIST_DIR)" $(if $(TARGET),--target "$(TARGET)")

package-all:
	GO="$(GO)" $(PYTHON) ./scripts/package/build.py package --all --version "$(VERSION)" --output "$(DIST_DIR)"

test-package:
	$(PYTHON) -m unittest discover -s scripts/package -p '*_test.py'

kc:
	$(GO) run ./cmd/kc -- $(ARGS)

serve:
	$(GO) run ./cmd/kc -- serve --config $(KC_DEPLOYMENT_CONFIG) --listen $(LISTEN)

typecheck:
	$(GO) test -run '^$$' ./...
