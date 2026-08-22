APP := stepanel
GO ?= go

.PHONY: all build test fmt vet check clean

all: check build

build:
	$(GO) build -trimpath -ldflags="-s -w" -o $(APP) .

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

check: fmt vet test

clean:
	rm -f $(APP) coverage.out
