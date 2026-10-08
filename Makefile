IMAGE = localhost/meknik-build
BIN = $(HOME)/.local/share/meknik/bin
APPS = $(HOME)/.local/share/applications
DESKTOP = tel.lil.meknik.desktop
IN_IMAGE = podman run --rm -v $(CURDIR):/src:Z -v meknik-gomod:/root/go/pkg/mod -v meknik-gocache:/root/.cache $(IMAGE)

.PHONY: image test lint build install

image:
	podman build -t $(IMAGE) .

test: image
	$(IN_IMAGE) go test ./...

lint: image
	$(IN_IMAGE) golangci-lint run ./...

build: image
	$(IN_IMAGE) env CGO_ENABLED=0 go build -buildvcs=false -o bin/meknik ./cmd/meknik
	$(IN_IMAGE) go build -buildvcs=false -o bin/meknik-ui ./cmd/meknik-ui

install: build
	install -D -m 755 -t $(BIN) bin/meknik bin/meknik-ui
	ln -sf $(BIN)/meknik $(HOME)/.local/bin/meknik
	ln -sf $(BIN)/meknik-ui $(HOME)/.local/bin/meknik-ui
	install -D -m 755 bin/meknik $(HOME)/.local/share/meknik/container-bin/meknik
	install -d $(APPS)
	sed 's|@BIN@|$(BIN)|' $(DESKTOP) > $(APPS)/$(DESKTOP)
	update-desktop-database $(APPS)
