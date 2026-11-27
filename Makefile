# `make hooks` turns on the git hooks in .githooks for this clone; see
# docs/agents/branching.md. `make dist` builds the release archives;
# .github/workflows/release.yml runs it for each v* tag. `make integration` runs arr against real Sonarr and
# Radarr in Docker; see docs/integration-tests.md. A plain `go test ./...`
# never runs those.

.PHONY: hooks
hooks:
	git config core.hooksPath .githooks

ARR_IT_SONARR_PORT ?= 18989
ARR_IT_RADARR_PORT ?= 17878
ARR_IT_SONARR_API_KEY ?= 0123456789abcdef0123456789abcdef
ARR_IT_RADARR_API_KEY ?= fedcba9876543210fedcba9876543210
ARR_IT_SONARR_URL ?= http://127.0.0.1:$(ARR_IT_SONARR_PORT)
ARR_IT_RADARR_URL ?= http://127.0.0.1:$(ARR_IT_RADARR_PORT)
export ARR_IT_SONARR_PORT ARR_IT_RADARR_PORT ARR_IT_SONARR_API_KEY ARR_IT_RADARR_API_KEY ARR_IT_SONARR_URL ARR_IT_RADARR_URL

# Extra flags for go test, as in make integration TESTFLAGS=-v.
TESTFLAGS ?=

COMPOSE = docker compose --file integration/compose.yaml --project-name arr-cli-integration

.PHONY: integration
# Start new Instances (removing any an interrupted run left), wait until both
# answer, run the integration tests, and stop and remove the Instances
# whatever the outcome, Ctrl-C included.
integration:
	@$(COMPOSE) down --volumes --remove-orphans --timeout 5 2>/dev/null; \
	trap '$(COMPOSE) down --volumes --remove-orphans' EXIT; \
	trap 'exit 130' INT TERM; \
	$(COMPOSE) up --detach --wait --wait-timeout 300 && \
	go test -tags integration -count=1 -run '^TestIntegration' $(TESTFLAGS) ./...

# The platforms a release is built for, as GOOS/GOARCH.
PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
# The release being built, as its tag; the workflow passes it. It is stamped
# into arr for arr --version, except dev, which leaves arr to report itself as
# a dev build (see internal/release).
VERSION ?= dev
LDFLAGS = -s -w$(if $(filter-out dev,$(VERSION)), -X github.com/drzippit/arr-cli/internal/release.tag=$(VERSION))

.PHONY: dist
# Build a static arr for each platform into dist/, each packed on its own as
# arr_<version>_<os>_<arch>.tar.gz (.zip for Windows), plus SHA256SUMS over
# the archives.
dist:
	@rm -rf dist && mkdir -p dist; \
	set -e; \
	for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		name=arr_$(VERSION)_$${os}_$${arch}; \
		bin=arr; [ "$$os" = windows ] && bin=arr.exe; \
		mkdir -p dist/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags='$(LDFLAGS)' -o dist/$$name/$$bin ./cmd/arr; \
		if [ "$$os" = windows ]; then \
			(cd dist/$$name && python3 -m zipfile -c ../$$name.zip $$bin); \
		else \
			tar -C dist/$$name -czf dist/$$name.tar.gz $$bin; \
		fi; \
		rm -rf dist/$$name; \
		echo "built dist/$$name"; \
	done; \
	cd dist && sha256sum arr_* > SHA256SUMS
