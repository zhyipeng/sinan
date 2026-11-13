#!/bin/sh
set -eu
# Compatibility entry point. Shared implementation supports all desktop targets.
exec node scripts/desktop.mjs prepare
