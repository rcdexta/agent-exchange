.PHONY: build test install
build:
	go build -o bin/ax ./cmd/ax
test:
	go test -race ./...
install: build
	mkdir -p "$(HOME)/.local/bin"
	install -m 755 bin/ax "$(HOME)/.local/bin/ax"
