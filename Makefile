TEST ?= ./...

default: build

build:
	go build

test:
	go test -v $(TEST) $(TESTARGS) -timeout=30s -parallel=4
	go test -race $(TEST) $(TESTARGS)

dist:
	@test -z $(GITHUB_TOKEN) || $(MAKE) goreleaser

tag:
	git tag $(TAG) && git push origin $(TAG)

goreleaser:
	go get github.com/goreleaser/goreleaser

dist:
	goreleaser --skip-validate --rm-dist

.PHONY: default dist test deps
