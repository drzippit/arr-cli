# Integration tests run arr against real Sonarr and Radarr in Docker; see
# docs/integration-tests.md. A plain `go test ./...` never runs them.

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
