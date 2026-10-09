XDG_DATA_HOME ?= $(HOME)/.local/share
BINDIR ?= $(HOME)/.local/bin
APPS = $(XDG_DATA_HOME)/applications
DESKTOP = io.github.chrchr.clanking.desktop
ICON = io.github.chrchr.clanking.svg
ICONS = $(XDG_DATA_HOME)/icons/hicolor/scalable/apps
SOURCES = $(shell find . -name '*.go') go.mod go.sum
GO_BUILD = go build -buildvcs=false

.PHONY: all test lint build install clean

all: build

test:
	go test ./...

lint:
	golangci-lint run ./...

build: bin/clanking bin/clanking-ui

bin/clanking: $(SOURCES)
	CGO_ENABLED=0 $(GO_BUILD) -o $@ ./cmd/clanking

bin/clanking-ui: $(SOURCES)
	$(GO_BUILD) -o $@ ./cmd/clanking-ui

install: build
	install -D -m 755 -t $(BINDIR) bin/clanking bin/clanking-ui
	install -D -m 644 -t $(ICONS) $(ICON)
	install -D -m 644 -t $(APPS) $(DESKTOP)
	update-desktop-database $(APPS)

clean:
	rm -rf bin
