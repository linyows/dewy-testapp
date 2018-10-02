TEST?=./...
NAME = "$(shell awk -F\" '/^const Name/ { print $$2; exit }' main.go)"
VERSION = "$(shell awk -F\" '/^const Version/ { print $$2; exit }' main.go)"
GOVERSION = "$(shell go version)"
GOENV = GO111MODULE=on

default: test

deps:
	$(GOENV) go get github.com/goreleaser/goreleaser

test: deps
	$(GOENV) go test -v $(TEST) $(TESTARGS) -timeout=30s -parallel=4
	$(GOENV) go test -race $(TEST) $(TESTARGS)

dist:
	@test -z $(GITHUB_TOKEN) || $(MAKE) goreleaser

goreleaser:
	git tag | grep v$(VERSION) || git tag v$(VERSION)
	git push origin v$(VERSION)
	GOVERSION=$(GOVERSION) goreleaser --rm-dist

.PHONY: default dist test deps
