.PHONY: build test vet fmt install uninstall

PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
VERSION ?= 2.0.0
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/curapi ./cmd/curapi

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './bin/*')

install: build
	install -d $(BINDIR)
	install -m 755 bin/curapi $(BINDIR)/curapi
	@echo "Installed $(BINDIR)/curapi"
	@echo "Next: curapi install   # register OS service (refreshes if present)"

uninstall:
	-$(BINDIR)/curapi uninstall
	-rm -f $(BINDIR)/curapi
	@echo "Removed $(BINDIR)/curapi"
