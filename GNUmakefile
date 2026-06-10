default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

generate:
	cd tools; go generate ./...

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

# Acceptance tests only — skips the unit-test packages entirely. Path mirrors CI
# (acceptance.yml) so a TestAcc* added outside internal/provider isn't silently dropped.
testacc-only:
	TF_ACC=1 go test -v -timeout 30m -run '^TestAcc' ./internal/...

.PHONY: fmt lint test testacc testacc-only build install generate
