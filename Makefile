TEST ?= ./...
VERSION ?= $(shell awk -F'"' '/\tversion.*=/ { print $$2; exit }' main.go)
GOVERSION ?= $(shell go version | awk '{ if (sub(/go version go/, "v")) print }' | awk '{print $$1 "-" $$2}')

default: test

test:
	go test -v $(TEST) $(TESTARGS) -timeout=30s -parallel=4
	go test -race $(TEST) $(TESTARGS)

dist:
	@test -z $(GITHUB_TOKEN) || $(MAKE) goreleaser

tag:
	git tag | grep v$(VERSION) || git tag v$(VERSION)
	git push origin v$(VERSION)

goreleaser:
	GO111MODULE=off go get github.com/goreleaser/goreleaser
	GOVERSION=$(GOVERSION) goreleaser --skip-publish

.PHONY: default dist test deps
