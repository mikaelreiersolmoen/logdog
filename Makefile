.PHONY: build run clean install test help

BINARY_NAME=logdog
BUILD_DIR=.
MAIN_PATH=main.go

help:
	@echo "Available commands:"
	@echo "  make build    - Build the binary"
	@echo "  make run      - Run the application (requires --app flag)"
	@echo "  make install  - Install the binary to GOPATH/bin"
	@echo "  make clean    - Remove the binary"
	@echo "  make test     - Run tests"

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

run:
	go run $(MAIN_PATH) $(ARGS)

install:
	go install

clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)

test:
	go test ./...
