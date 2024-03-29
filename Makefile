GITV != git describe --tags
GITC != git rev-parse --verify HEAD
SRC  != find . -type f -name '*.go' -or -name '*.html' ! -name '*_test.go'
TEST != find . -type f -name '*_test.go'

PREFIX  ?= /usr/local
VERSION ?= $(GITV)
COMMIT  ?= $(GITC)
BUILDER ?= Makefile

GO      := go
INSTALL := install
RM      := rm

lannet: go.mod go.sum $(SRC)
	GO111MODULE=on CGO_ENABLED=0 $(GO) build -o $@ -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.builtBy=$(BUILDER)"

.PHONY: clean
clean:
	$(RM) -f lannet

.PHONY: install
install: lannet
	install -d $(PREFIX)/bin/
	install -m 755 lannet $(PREFIX)/bin/lannet

.PHONY: uninstall
uninstall:
	$(RM) -f $(PREFIX)/bin/lannet

# Development helpers
.PHONY: fmt
fmt:
	go fmt ./...
