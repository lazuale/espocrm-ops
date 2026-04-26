SHELL := bash
APP := espops
BIN := bin/$(APP)

.PHONY: build test ci fmt vet coverage clean

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/espops

test:
	go test ./...

ci: test

fmt:
	go fmt ./...

vet:
	go vet ./...

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

clean:
	rm -rf bin coverage.out
