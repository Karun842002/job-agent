.PHONY: build test

build:
	go build -mod=readonly ./...

test:
	go test -mod=readonly ./...
