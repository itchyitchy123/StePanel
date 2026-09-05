APP := stepanel
GO ?= go
LDFLAGS := -s -w -X main.Commit=$${GIT_COMMIT:-dev} -X main.BuildDate=$$(date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: all build test fmt fmt-check vet coverage check release-check clean

all: check build

build:
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(APP) .

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$($(GO)fmt -l .)"

coverage:
	$(GO) test ./... -coverprofile=coverage.out -covermode=atomic

vet:
	$(GO) vet ./...

check: fmt-check vet test

release-check:
	./scripts/check-release.sh

clean:
	rm -f $(APP) coverage.out
