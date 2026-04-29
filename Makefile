.PHONY: build test fmt vet daemon

GO := go

build: daemon

daemon:
	$(GO) build -o bin/relayd ./cmd/relayd

test:
	$(GO) test -count=1 ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...
