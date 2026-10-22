#!/bin/sh
# Require the branch's Go minimum, including the CVE-2026-111943 mitigation.
set -eu

GO=${GO:-go}
version=$("$GO" env GOVERSION)
numbers=$(printf '%s\n' "$version" | sed -n 's/^go\([0-9][0-9]*\)\.\([0-9][0-9]*\)\.\([0-9][0-9]*\)$/\1 \2 \3/p')
# Splitting the validated numeric version is intentional.
# shellcheck disable=SC2086
set -- $numbers
if [ "$#" -eq 3 ]; then
    if [ "$1" -eq 1 ] && {
        { [ "$2" -eq 27 ] && [ "$3" -ge 3 ]; } ||
        [ "$2" -gt 27 ];
    }; then
        exit 0
    fi
fi
printf 'Unsupported Go compiler %s: use stable Go 1.27.3 or a later release (CVE-2026-111943).\n' "$version" >&2
exit 1
