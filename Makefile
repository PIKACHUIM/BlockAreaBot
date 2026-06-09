BINARY_NAME=block
VERSION?=0.1.0
BUILD_DIR=build
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

GO=go
GOFLAGS=-trimpath

.PHONY: all build clean linux-amd64 linux-arm64

all: linux-amd64 linux-arm64

build:
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .

clean:
	rm -rf $(BUILD_DIR)

install: build
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	install -Dm644 dist/block-area-bot.service /etc/systemd/system/block-area-bot.service
	install -Dm644 dist/config.json /etc/block-area-bot/config.json
	mkdir -p /var/lib/block-area-bot
	mkdir -p /var/log/block-area-bot
	systemctl daemon-reload

uninstall:
	systemctl stop block-area-bot || true
	systemctl disable block-area-bot || true
	rm -f /usr/local/bin/$(BINARY_NAME)
	rm -f /etc/systemd/system/block-area-bot.service
	systemctl daemon-reload
