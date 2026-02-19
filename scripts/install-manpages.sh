#!/usr/bin/env bash
# scripts/install-manpages.sh
# Install Raven man pages to the system man directory.
#
# Usage:
#   ./scripts/install-manpages.sh [mandir]
#
# The optional argument overrides the default install location.
# Default: /usr/local/share/man/man1
#
# Examples:
#   sudo ./scripts/install-manpages.sh                 # system-wide install
#   ./scripts/install-manpages.sh ~/.local/share/man/man1  # user install

set -euo pipefail

MANDIR="${1:-/usr/local/share/man/man1}"
SRCDIR="man/man1"

if [[ ! -d "$SRCDIR" ]]; then
    echo "Error: man pages not found in $SRCDIR." >&2
    echo "Run 'make manpages' first to generate them." >&2
    exit 1
fi

shopt -s nullglob
files=("$SRCDIR"/*.1)

if [[ ${#files[@]} -eq 0 ]]; then
    echo "Error: no .1 files found in $SRCDIR." >&2
    exit 1
fi

mkdir -p "$MANDIR"

for f in "${files[@]}"; do
    cp "$f" "$MANDIR/"
    echo "Installed: $MANDIR/$(basename "$f")"
done

# Update the man database when possible.
if command -v mandb > /dev/null 2>&1; then
    mandb --quiet || true
elif command -v makewhatis > /dev/null 2>&1; then
    makewhatis "$MANDIR" || true
fi

echo ""
echo "Man pages installed to $MANDIR"
echo "Try: man raven"
