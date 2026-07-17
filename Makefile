.PHONY: all build force install dev test test-dom lint security install-hooks clean

all: build

build: test
	go build -o vane.exe .

force:
	go build -o vane.exe .

install:
	go install .

dev:
	go run . run $(DIR)

test:
	go test ./...

# Runs core/ and core/router/ DOM code (js/wasm build tag) against a jsdom
# DOM via a Node.js exec wrapper, no real browser needed. First run installs
# the jsdom devDependency (tools/wasmtest/). See internal_docs/testing.md.
# Known gap: jsdom has no layout engine, so anything depending on real
# layout (offsetWidth, etc.) isn't testable this way.
test-dom:
	cd tools/wasmtest && npm install --silent
	GOOS=js GOARCH=wasm go test -exec="node $(CURDIR)/tools/wasmtest/wasm_test_exec.js" ./core/... ./core/router/...

lint:
	golangci-lint run

security:
	gosec ./...

# One-time setup so `git push` runs the CI workflow locally first (via act +
# Docker). See .githooks/pre-push. Safe to re-run.
install-hooks:
	git config core.hooksPath .githooks

clean:
	del /f vane.exe 2>nul || true
