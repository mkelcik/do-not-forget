# Image for Anthropic's OSS Scanner (github.com/anthropics/oss-scanner). The audit runs offline, so this
# fetches every module and builds the app and its tests. threat_model.md, next to this file, says what matters.

# The Go release go.mod asks for; trixie ships the GTK4 / WebKitGTK 6.0 headers Wails needs for cgo on Linux.
FROM golang:1.27-trixie
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends libgtk-4-dev libwebkitgtk-6.0-dev \
    && rm -rf /var/lib/apt/lists/*

# scanner contract: the checkout lives inside the image, at /src
COPY . /src
WORKDIR /src

# The app (as `make build` and the release build it) and its tests, with and without -race as `make test` runs them.
RUN go mod download \
    && go build -o do-not-forget . \
    && go build -tags production -o /tmp/do-not-forget-production . \
    && go vet ./... \
    && go test -run '^$' ./... \
    && go test -race -run '^$' ./...

# Run the tests so the log shows the image works, without letting a failure abort the build.
RUN timeout 900 go test -race -count=1 ./... || echo "WARNING: go test reported failures"

# Everything is fetched. From here on the go command must not reach a proxy or download a toolchain.
ENV GOPROXY=off \
    GOTOOLCHAIN=local
