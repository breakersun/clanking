.PHONY: test lint install

test:
	go test ./...

lint:
	golangci-lint run ./...

install:
	GOBIN=$(HOME)/.local/bin go install ./cmd/meknik
