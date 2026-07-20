#!/bin/bash
# Finder launches this with no arguments and an unpredictable working
# directory, so resolve the bundle's own location to find the real binary
# next to this script, then always launch it in GUI mode — there's no way
# to pass CLI flags via a double-click anyway.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$DIR/fberadicator-bin" -gui
