GO ?= go
GOFMT = $(shell $(GO) env GOROOT)/bin/gofmt
TARGET_ARCH ?= arm64

.PHONY: check test generate format build integration resources clean

generate:
	cd routeguard/src && $(GO) generate ./...

test:
	cd routeguard/src && $(GO) test ./...
	npm run test:ui
	python3 -B -m unittest discover -s tests/release

check: test
	cd routeguard/src && $(GO) vet ./...
	unformatted="$$('$(GOFMT)' -l routeguard/src)" && test -z "$$unformatted"
	npm run format:check
	sh scripts/check-shell.sh

format:
	"$(GOFMT)" -w routeguard/src
	npm run format

build:
	mkdir -p build
	cd routeguard/src && CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGET_ARCH) $(GO) build -trimpath -buildvcs=false -ldflags='-s -w' -o ../../build/routeguard ./cmd/routeguard

integration: build
	docker build -t routeguard-test:local -f tests/integration/Dockerfile .
	docker run --rm --privileged --network=none -v "$(CURDIR):/src:ro" -v "$(CURDIR)/build:/out" routeguard-test:local sh tests/integration/run.sh

resources: build
	docker build -t routeguard-test:local -f tests/integration/Dockerfile .
	docker run --rm --privileged --network=none -v "$(CURDIR):/src:ro" -v "$(CURDIR)/build:/out" routeguard-test:local sh tests/integration/resources.sh

clean:
	rm -rf build
