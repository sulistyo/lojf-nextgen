APP=lojf-nextgen
PKG=./cmd/server

.PHONY: all tidy build run test clean fmt seed

all: build

tidy:
	go mod tidy

fmt:
	go fmt ./...

build: tidy
	CGO_ENABLED=1 go build -ldflags "-X github.com/lojf/nextgen/internal/handlers.BuildVersion=$(shell git rev-parse --short HEAD)" -o bin/$(APP) $(PKG)

run:
	./bin/$(APP)

test:
	go test ./... -v

clean:
	rm -rf bin

seed:
	go run ./cmd/seed --reset
