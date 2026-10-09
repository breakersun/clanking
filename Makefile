IMAGE = localhost/clanking-build
XDG_DATA_HOME ?= $(HOME)/.local/share
BINDIR ?= $(HOME)/.local/bin
APPS = $(XDG_DATA_HOME)/applications
DESKTOP = io.github.chrchr.clanking.desktop
ICON = io.github.chrchr.clanking.svg
ICONS = $(XDG_DATA_HOME)/icons/hicolor/scalable/apps
IN_IMAGE = podman run --rm -v $(CURDIR):/src:Z -v clanking-gomod:/root/go/pkg/mod -v clanking-gocache:/root/.cache $(IMAGE)

.PHONY: all image test lint build install clean

all: build

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
	install -D -m 755 -t $(BINDIR) bin/clanking bin/clanking-ui
	install -D -m 644 -t $(ICONS) $(ICON)
	install -D -m 644 -t $(APPS) $(DESKTOP)
	update-desktop-database $(APPS)

clean:
	rm -rf bin
