TEST ?= ./...

default: build

build:
	go build

test:
	go test -v $(TEST) $(TESTARGS) -timeout=30s -parallel=4
	go test -race $(TEST) $(TESTARGS)

tag:
	git tag $(TAG) && git push origin $(TAG)

goreleaser:
	go get github.com/goreleaser/goreleaser

release:
	@test -z $(GITHUB_TOKEN) || goreleaser release --clean

dist:
	goreleaser r --clean --snapshot

.PHONY: default dist test deps
