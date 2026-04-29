.PHONY: build test fmt check

build:
	test -d bin || mkdir bin
	go build -o bin/espops .

test:
	go test ./...

fmt:
	gofmt -w *.go

check:
	test -z "$$(gofmt -l *.go)"
	go test ./...
	test -d bin || mkdir bin
	go build -o bin/espops .
	! grep -R '"bash"\|"sh"\|"-c"\|sha256sum\|tar \|gzip \|rm -rf\|mkdir -p' --include='*.go' .
