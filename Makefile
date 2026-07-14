.PHONY: test
test:
	CGO_ENABLED=0 go test ./... $(TESTARGS)

.PHONY: smoke-test
smoke-test:
	./scripts/cpanel-smoke.sh

.PHONY: test-acceptance
test-acceptance:
	./scripts/test-acceptance.sh $(TESTARGS)

.PHONY: generate-documentation
generate-documentation:
	go generate ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: plan
plan:
	terraform plan -parallelism=1

.PHONY: apply
apply:
	terraform apply -parallelism=1
