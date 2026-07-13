.PHONY: build run test vet fmt lint tidy clean
BINARY := bin/lazygh

build:
	go build -o $(BINARY) ./cmd/lazygh

run:
	go run ./cmd/lazygh

test:
	go test ./...

vet:
	go vet ./...

fmt:
	golangci-lint fmt

lint:
	golangci-lint run

tidy:
	go mod tidy

fix:
	go fix ./...

clean:
	rm -rf bin
