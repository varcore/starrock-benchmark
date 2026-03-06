#!/bin/bash
set -euo pipefail

BUMP_TYPE="${1:?Usage: bump-version.sh <patch|minor|major>}"
VERSION_FILE="$(cd "$(dirname "$0")/.." && pwd)/VERSION"

CURRENT=$(cat "$VERSION_FILE" | tr -d '[:space:]')
IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT"

case "$BUMP_TYPE" in
    patch) PATCH=$((PATCH + 1)) ;;
    minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
    major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
    *) echo "Invalid bump type: $BUMP_TYPE (use patch, minor, or major)"; exit 1 ;;
esac

NEW_VERSION="${MAJOR}.${MINOR}.${PATCH}"
echo "$NEW_VERSION" > "$VERSION_FILE"

echo "Version bumped: $CURRENT -> $NEW_VERSION"
