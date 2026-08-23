APP := stepanel
GO ?= go
LDFLAGS := -s -w -X main.Commit=$${GIT_COMMIT:-dev} -X main.BuildDate=$$(date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: all build test fmt vet check clean

all: check build

build:
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(APP) .

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

check: fmt vet test

clean:
	rm -f $(APP) coverage.out
