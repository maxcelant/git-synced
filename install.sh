#!/usr/bin/env bash
set -euo pipefail

BINARY="git-synced"
INSTALL_DIR="/usr/local/bin"

echo "Building $BINARY..."
go build -o "$BINARY" ./cmd/main.go

echo "Installing to $INSTALL_DIR/$BINARY..."
sudo mv "$BINARY" "$INSTALL_DIR/$BINARY"

CONFIG_DIR="$HOME/.git-synced"
CONFIG_FILE="$CONFIG_DIR/config.yaml"
if [ ! -f "$CONFIG_FILE" ]; then
  echo "Creating config at $CONFIG_FILE..."
  mkdir -p "$CONFIG_DIR"
  cp config.yaml "$CONFIG_FILE"
  echo "Edit $CONFIG_FILE to add your token and repos."
else
  echo "Config already exists at $CONFIG_FILE, skipping."
fi

echo "Done. Run: $BINARY --help"
