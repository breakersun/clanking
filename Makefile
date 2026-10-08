IMAGE = localhost/clanking-build
BIN = $(HOME)/.local/share/clanking/bin
APPS = $(HOME)/.local/share/applications
DESKTOP = io.github.chrchr.clanking.desktop
ICON = io.github.chrchr.clanking.svg
ICONS = $(HOME)/.local/share/icons/hicolor/scalable/apps
IN_IMAGE = podman run --rm -v $(CURDIR):/src:Z -v clanking-gomod:/root/go/pkg/mod -v clanking-gocache:/root/.cache $(IMAGE)

.PHONY: image test lint build install

image:
	podman build -t $(IMAGE) .

test: image
	$(IN_IMAGE) go test ./...

lint: image
	$(IN_IMAGE) golangci-lint run ./...

build: image
	$(IN_IMAGE) env CGO_ENABLED=0 go build -buildvcs=false -o bin/clanking ./cmd/clanking
	$(IN_IMAGE) go build -buildvcs=false -o bin/clanking-ui ./cmd/clanking-ui

install: build
	install -D -m 755 -t $(BIN) bin/clanking bin/clanking-ui
	ln -sf $(BIN)/clanking $(HOME)/.local/bin/clanking
	ln -sf $(BIN)/clanking-ui $(HOME)/.local/bin/clanking-ui
	install -D -m 755 bin/clanking $(HOME)/.local/share/clanking/container-bin/clanking
	install -D -m 644 -t $(ICONS) $(ICON)
	install -d $(APPS)
	sed 's|@BIN@|$(BIN)|' $(DESKTOP) > $(APPS)/$(DESKTOP).tmp
	mv $(APPS)/$(DESKTOP).tmp $(APPS)/$(DESKTOP)
	update-desktop-database $(APPS)
