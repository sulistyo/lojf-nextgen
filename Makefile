APP=lojf-nextgen
PKG=./cmd/server

.PHONY: all tidy build run test clean fmt

all: build

tidy:
	go mod tidy

fmt:
	go fmt ./...

build: tidy
	CGO_ENABLED=1 go build -o bin/$(APP) $(PKG)

run:
	./bin/$(APP)

test:
	go test ./... -v

clean:
	rm -rf bin
