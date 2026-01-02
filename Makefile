.PHONY: build run clean install test help build-macos build-linux build-windows build-all

BINARY_NAME=logdog
BUILD_DIR=.
DIST_DIR=build
MAIN_PATH=main.go

help:
	@echo "Available commands:"
	@echo "  make build    - Build the binary"
	@echo "  make run      - Run the application (requires --app flag)"
	@echo "  make install  - Install the binary to GOPATH/bin"
	@echo "  make clean    - Remove the binary"
	@echo "  make test     - Run tests"
	@echo "  make build-macos - Build macOS Intel and Apple Silicon binaries"
	@echo "  make build-linux - Build Linux amd64 and arm64 binaries"
	@echo "  make build-windows - Build Windows amd64 and arm64 binaries"
	@echo "  make build-all - Build macOS, Linux, and Windows binaries"

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

build-macos:
	mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 go build -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	tar -czf $(DIST_DIR)/$(BINARY_NAME)-macos.tar.gz -C $(DIST_DIR) $(BINARY_NAME)-darwin-amd64 $(BINARY_NAME)-darwin-arm64

build-linux:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm64 go build -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	tar -czf $(DIST_DIR)/$(BINARY_NAME)-linux.tar.gz -C $(DIST_DIR) $(BINARY_NAME)-linux-amd64 $(BINARY_NAME)-linux-arm64

build-windows:
	mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 go build -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	GOOS=windows GOARCH=arm64 go build -o $(DIST_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_PATH)
	cd $(DIST_DIR) && zip -q $(BINARY_NAME)-windows.zip $(BINARY_NAME)-windows-amd64.exe $(BINARY_NAME)-windows-arm64.exe

build-all: build-macos build-linux build-windows

run:
	go run $(MAIN_PATH) $(ARGS)

install:
	go install

clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)

test:
	go test ./...
