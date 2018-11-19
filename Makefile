TEST ?= ./...
TAG ?= $(shell git fetch --tags && git describe --tags `git rev-list --tags --max-count=1`)

default: test

test:
	go test -v $(TEST) $(TESTARGS) -timeout=30s -parallel=4
	go test -race $(TEST) $(TESTARGS)

dist:
	@test -z $(GITHUB_TOKEN) || $(MAKE) goreleaser

tag:
	@echo $(TAG)

goreleaser:
	GO111MODULE=off go get github.com/goreleaser/goreleaser
	goreleaser --skip-publish --snapshot

.PHONY: default dist test deps
