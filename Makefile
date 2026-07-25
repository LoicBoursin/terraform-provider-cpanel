.DEFAULT_GOAL := test

TOOLS_DIR ?= $(CURDIR)/.git/tools

.PHONY: test
test:
	CGO_ENABLED=0 go test ./... $(TESTARGS)

.PHONY: test-race
test-race:
	CGO_ENABLED=0 go test -race ./... $(TESTARGS)

.PHONY: smoke-test
smoke-test:
	./scripts/cpanel-smoke.sh

.PHONY: clean-acceptance
clean-acceptance:
	./scripts/cpanel-restore-test-singletons.sh
	./scripts/cpanel-clean-test-artifacts.sh
	CPANEL_REQUIRE_EMPTY=1 ./scripts/cpanel-smoke.sh

.PHONY: test-acceptance
test-acceptance:
	./scripts/test-acceptance.sh $(TESTARGS)

.PHONY: generate-documentation
generate-documentation:
	go generate ./...

.PHONY: tools
tools:
	TOOLS_DIR="$(TOOLS_DIR)" ./scripts/install-tools.sh

.PHONY: check-tools
check-tools:
	@TOOLS_DIR="$(TOOLS_DIR)" ./scripts/check-tools.sh

.PHONY: check-release-tools
check-release-tools:
	@TOOLS_DIR="$(TOOLS_DIR)" ./scripts/check-tools.sh --release

.PHONY: lint
lint: check-tools
	$(TOOLS_DIR)/golangci-lint run ./...

.PHONY: verify
verify: check-tools
	go mod tidy -diff
	go mod verify
	CGO_ENABLED=0 go build ./...
	CGO_ENABLED=0 go test -race ./...
	go vet ./...
	$(TOOLS_DIR)/golangci-lint run ./...
	$(TOOLS_DIR)/govulncheck ./...
	$(TOOLS_DIR)/actionlint .github/workflows/*.yml
	bash -n scripts/*.sh
	$(TOOLS_DIR)/shellcheck -x -P scripts scripts/*.sh
	./scripts/test-cpanel-common.sh
	./scripts/test-cpanel-ci-acceptance.sh
	./scripts/test-cpanel-clean-api-tokens.sh
	./scripts/test-cpanel-verify-test-account.sh
	./scripts/test-validate-release-tag.sh
	./scripts/test-materialize-release-manifest.sh
	./scripts/test-verify-published-release.sh
	$(TOOLS_DIR)/gitleaks dir --no-banner --redact .
	$(TOOLS_DIR)/gitleaks git --no-banner --redact .
	go generate ./...
	git diff --compact-summary --exit-code
	$(TOOLS_DIR)/goreleaser check

.PHONY: release-snapshot
release-snapshot: check-release-tools
	PATH="$(TOOLS_DIR):$$PATH" $(TOOLS_DIR)/goreleaser release --snapshot --clean --skip=sign
	./scripts/materialize-release-manifest.sh
	./scripts/verify-release-snapshot.sh

.PHONY: release-reproducibility
release-reproducibility: check-release-tools
	TOOLS_DIR="$(TOOLS_DIR)" ./scripts/verify-release-reproducibility.sh

.PHONY: plan
plan:
	terraform plan

.PHONY: apply
apply:
	terraform apply
