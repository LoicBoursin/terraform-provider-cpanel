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
	./scripts/cpanel-clean-test-artifacts.sh

.PHONY: test-acceptance
test-acceptance:
	./scripts/test-acceptance.sh $(TESTARGS)

.PHONY: generate-documentation
generate-documentation:
	go generate ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: verify
verify:
	go mod verify
	CGO_ENABLED=0 go test -race ./...
	go vet ./...
	golangci-lint run ./...
	govulncheck ./...
	actionlint .github/workflows/*.yml
	bash -n scripts/*.sh
	go generate ./...
	git diff --compact-summary --exit-code
	goreleaser check

.PHONY: release-snapshot
release-snapshot:
	goreleaser release --snapshot --clean --skip=sign

.PHONY: plan
plan:
	terraform plan

.PHONY: apply
apply:
	terraform apply
