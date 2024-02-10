# Run acceptance tests
.PHONY: test
test:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

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

