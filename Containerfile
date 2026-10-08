FROM docker.io/library/debian:13.7

RUN echo 'deb http://deb.debian.org/debian trixie-backports main' > /etc/apt/sources.list.d/backports.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends build-essential ca-certificates git pkg-config libgtk-4-dev libgirepository1.0-dev \
    && apt-get install -y --no-install-recommends -t trixie-backports golang-1.26-go \
    && rm -rf /var/lib/apt/lists/*

ENV PATH=/usr/lib/go-1.26/bin:$PATH \
    GOTOOLCHAIN=local

RUN GOBIN=/usr/local/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

WORKDIR /src
