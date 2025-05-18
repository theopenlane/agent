#!/bin/bash

set -e

VERSION="0.16.0"
BASE_URL="https://github.com/charmbracelet/gum/releases/download/v$VERSION"
BINARY_NAME="gum"

OS=$(uname -s)

case "$OS" in
Linux)
	PLATFORM="Linux"
	;;
Darwin)
	PLATFORM="Darwin"
	;;
*)
	echo "❌ Unsupported OS: $OS"
	exit 1
	;;
esac

if command -v $BINARY_NAME >/dev/null 2>&1; then
	echo "✅ '$BINARY_NAME' is already installed at $(which $BINARY_NAME)"
	exit 0
fi

ARCH=$(uname -m)

case "$ARCH" in
x86_64)
	ARCH_LABEL="x86_64"
	;;
arm64 | aarch64)
	ARCH_LABEL="arm64"
	;;
armv7l)
	ARCH_LABEL="armv7"
	;;
i386)
	ARCH_LABEL="386"
	;;
*)
	echo "❌ Unsupported architecture: $ARCH"
	exit 1
	;;
esac

ARCHIVE_FILE="gum_${VERSION}_${PLATFORM}_${ARCH_LABEL}.tar.gz"
EXTRACTED_BINARY="gum_${VERSION}_${PLATFORM}_${ARCH_LABEL}"

echo "Downloading $ARCHIVE_FILE..."
curl -L -o "$ARCHIVE_FILE" "$BASE_URL/$ARCHIVE_FILE"

echo "Extracting $ARCHIVE_FILE..."
tar -xzf "$ARCHIVE_FILE"

echo "'$BINARY_NAME' is ready to use in the current directory."

NAME=$(./gum/gum input --placeholder "Name of your runner")

TAGS=$(./gum/gum input --placeholder "Your tags in csv format e.g linux,k8s")

IP_ADDRESS=$(curl -s https://api.ipify.org)

echo $TAGS
echo $NAME
echo $IP_ADDRESS

RESPONSE=$(./gum/gum spin --spinner dot --title "Registering Runner" -- curl -X POST http://localhost:17608/v1/runners \
	-H "Content-Type: application/json" \
	-d "$(jq -n \
		--arg tags "$TAGS" \
		--arg name "$NAME" \
		--arg ip "$IP_ADDRESS" \
		'{tags: $tags, name: $name, ip_address: $ip}')")

echo $RESPONSE
