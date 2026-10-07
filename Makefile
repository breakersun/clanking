.PHONY: test lint install

test:
	go test ./...

lint:
	golangci-lint run ./...

install:
	GOBIN=$(HOME)/.local/share/meknik/bin go install -buildvcs=false ./cmd/meknik
	ln -sf $(HOME)/.local/share/meknik/bin/meknik $(HOME)/.local/bin/meknik
	install -D -m 755 $(HOME)/.local/share/meknik/bin/meknik $(HOME)/.local/share/meknik/container-bin/meknik
