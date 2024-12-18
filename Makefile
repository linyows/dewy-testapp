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

release: goreleaser
	@test -z $(GITHUB_TOKEN) || goreleaser --rm-dist --skip-validate

dist:
	goreleaser --snapshot --skip-publish --rm-dist

.PHONY: default dist test deps
