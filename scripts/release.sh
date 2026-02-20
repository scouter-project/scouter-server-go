#!/usr/bin/env bash
set -euo pipefail

# Usage: ./scripts/release.sh v2.21.0 [--draft] [--prerelease]
#
# Creates a GitHub release with cross-platform distribution packages.
# Prerequisites: gh (GitHub CLI) authenticated, make, go

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# --- Parse arguments ---
VERSION=""
DRAFT=""
PRERELEASE=""

for arg in "$@"; do
    case "$arg" in
        --draft)      DRAFT="--draft" ;;
        --prerelease) PRERELEASE="--prerelease" ;;
        v*)           VERSION="$arg" ;;
        *)            echo "Unknown argument: $arg"; exit 1 ;;
    esac
done

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version> [--draft] [--prerelease]"
    echo "  version     : Tag name, e.g. v2.21.0"
    echo "  --draft     : Create as draft release"
    echo "  --prerelease: Mark as pre-release"
    exit 1
fi

# --- Preflight checks ---
if ! command -v gh &>/dev/null; then
    echo "Error: gh (GitHub CLI) is not installed."
    exit 1
fi

if ! gh auth status &>/dev/null; then
    echo "Error: gh is not authenticated. Run 'gh auth login' first."
    exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
    echo "Error: Working directory is not clean. Commit or stash changes first."
    exit 1
fi

# --- Confirm ---
echo "=== Scouter Server Go Release ==="
echo "  Version : $VERSION"
echo "  Branch  : $(git branch --show-current)"
echo "  Commit  : $(git rev-parse --short HEAD)"
[ -n "$DRAFT" ]      && echo "  Draft   : yes"
[ -n "$PRERELEASE" ] && echo "  Pre-rel : yes"
echo ""
read -rp "Proceed? [y/N] " confirm
if [[ ! "$confirm" =~ ^[yY]$ ]]; then
    echo "Aborted."
    exit 0
fi

# --- Tag ---
if git rev-parse "$VERSION" &>/dev/null; then
    echo "Tag $VERSION already exists. Using existing tag."
else
    echo "Creating tag $VERSION ..."
    git tag -a "$VERSION" -m "Release $VERSION"
    git push origin "$VERSION"
fi

# --- Build distribution packages ---
echo "Building distribution packages ..."
make dist-all

# --- Collect assets ---
ASSETS=()
for f in dist/*.tar.gz dist/*.zip; do
    [ -f "$f" ] && ASSETS+=("$f")
done

if [ ${#ASSETS[@]} -eq 0 ]; then
    echo "Error: No distribution packages found in dist/"
    exit 1
fi

echo "Assets to upload:"
for f in "${ASSETS[@]}"; do
    echo "  $(basename "$f")  ($(du -h "$f" | cut -f1))"
done

# --- Generate release notes from git log ---
PREV_TAG=$(git tag --sort=-v:refname | grep -v "^$VERSION$" | head -1 || true)

if [ -n "$PREV_TAG" ]; then
    NOTES=$(git log --pretty=format:"- %s" "$PREV_TAG".."$VERSION" 2>/dev/null || echo "")
    NOTES_TITLE="## Changes since $PREV_TAG"
else
    NOTES=$(git log --pretty=format:"- %s" --max-count=20)
    NOTES_TITLE="## Recent Changes"
fi

RELEASE_BODY="$(cat <<EOF
$NOTES_TITLE

$NOTES

## Assets

| File | Platform |
|------|----------|
| scouter-server-linux-amd64.tar.gz | Linux x86_64 |
| scouter-server-linux-arm64.tar.gz | Linux ARM64 |
| scouter-server-darwin-amd64.tar.gz | macOS x86_64 |
| scouter-server-darwin-arm64.tar.gz | macOS ARM64 (Apple Silicon) |
| scouter-server-windows-amd64.zip | Windows x86_64 |
EOF
)"

# --- Create GitHub release ---
echo ""
echo "Creating GitHub release $VERSION ..."

gh release create "$VERSION" \
    --title "$VERSION" \
    --notes "$RELEASE_BODY" \
    $DRAFT \
    $PRERELEASE \
    "${ASSETS[@]}"

echo ""
echo "Release created: $(gh release view "$VERSION" --json url -q '.url')"
