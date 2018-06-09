TEST?=./...
NAME = "$(shell awk -F\" '/^const Name/ { print $$2; exit }' version.go)"
VERSION = "$(shell awk -F\" '/^const Version/ { print $$2; exit }' version.go)"
GOVERSION = "$(shell go version)"

default: test

deps:
	brew install goreleaser/tap/goreleaser

test: deps
	go test -v $(TEST) $(TESTARGS) -timeout=30s -parallel=4
	go test -race $(TEST) $(TESTARGS)

dist:
	@test -z $(GITHUB_TOKEN) || $(MAKE) goreleaser

goreleaser:
	git tag | grep v$(VERSION) || git tag v$(VERSION)
	git push origin v$(VERSION)
	env $(GOVERSION) goreleaser --rm-dist

.PHONY: default dist test deps
