#!/usr/bin/env bash
# validate.sh - build-checks every plugin in the plugins/ directory.
# Run from the repository root: ./tools/validate.sh
# Exit code is 0 only if all plugins build cleanly.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLUGINS_DIR="$REPO_ROOT/plugins"

pass=0
fail=0
errors=()

for plugin_dir in "$PLUGINS_DIR"/*/; do
    name="$(basename "$plugin_dir")"
    if [ ! -f "$plugin_dir/go.mod" ]; then
        echo "SKIP $name (no go.mod)"
        continue
    fi

    echo -n "Building $name ... "
    if (cd "$plugin_dir" && go build ./... 2>&1); then
        echo "OK"
        pass=$((pass + 1))
    else
        echo "FAIL"
        fail=$((fail + 1))
        errors+=("$name")
    fi
done

echo ""
echo "Results: $pass passed, $fail failed"

if [ $fail -gt 0 ]; then
    echo "Failed plugins: ${errors[*]}"
    exit 1
fi

exit 0
