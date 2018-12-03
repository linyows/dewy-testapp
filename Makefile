TEST ?= ./...

LTAG ?= $(shell git fetch --tags && git describe --tags `git rev-list --tags --max-count=1`)
LMMP ?= $(shell echo $(LTAG) | cut -d'-' -f1)
LPRE ?= $(shell echo $(LTAG) | cut -d'-' -f2 | awk -F . '{print $$2}')
LPATCH ?= $(shell echo $(LMMP) | cut -d'.' -f3)
TAG ?= $(shell echo $(LMMP) | cut -d'.' -f1 -f2).$(shell expr $(LPATCH) + 1)
PRETAG ?= $(TAG)-$(USER).$(shell test -z $(LPRE) && echo 0 || expr $(LPRE) + 1)

default: test

test:
	go test -v $(TEST) $(TESTARGS) -timeout=30s -parallel=4
	go test -race $(TEST) $(TESTARGS)

dist:
	@test -z $(GITHUB_TOKEN) || $(MAKE) goreleaser

tag:
	git tag $(TAG) && git push origin $(TAG)

pretag:
	git tag $(PRETAG) && git push origin $(PRETAG)

goreleaser: export GO111MODULE=off
goreleaser:
	go get github.com/goreleaser/goreleaser
	goreleaser --skip-validate --rm-dist

.PHONY: default dist test deps
