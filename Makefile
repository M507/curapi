.PHONY: build test vet fmt install uninstall ensure-path

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
	@$(MAKE) --no-print-directory ensure-path
	@echo "Next: curapi install   # register OS service (refreshes if present)"

# Put BINDIR on PATH when this shell does not already have it.
# If the directory is missing from PATH, append an export to the user's
# shell rc (.zshrc, .bashrc, or .profile) unless that file already
# mentions BINDIR.
ensure-path:
	@bindir="$(BINDIR)"; \
	case ":$(PATH):" in \
	*":$$bindir:"*) \
		echo "$$bindir is already on PATH" ;; \
	*) \
		rc="$(HOME)/.profile"; \
		shellname=`basename "$${SHELL:-sh}"`; \
		if [ "$$shellname" = zsh ]; then \
			rc="$(HOME)/.zshrc"; \
		elif [ -f "$(HOME)/.bashrc" ]; then \
			rc="$(HOME)/.bashrc"; \
		fi; \
		if [ -f "$$rc" ] && grep -F "$$bindir" "$$rc" >/dev/null 2>&1; then \
			echo "$$bindir is listed in $$rc but not in this shell PATH"; \
			echo "Run: export PATH=\"$$bindir:\$$PATH\""; \
		else \
			touch "$$rc"; \
			printf '\n# added by curapi make install\nexport PATH="%s:$$PATH"\n' "$$bindir" >> "$$rc"; \
			echo "Added $$bindir to PATH in $$rc"; \
			echo "This shell: export PATH=\"$$bindir:\$$PATH\""; \
		fi ;; \
	esac

uninstall:
	-$(BINDIR)/curapi uninstall
	-rm -f $(BINDIR)/curapi
	@echo "Removed $(BINDIR)/curapi"
