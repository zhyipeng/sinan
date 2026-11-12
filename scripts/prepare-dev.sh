#!/bin/sh
set -eu

if [ "$(go env GOOS)" = darwin ]; then
  mkdir -p .sinan-dev/packages/app-search/backend
  cp plugins/system-app-search/manifest.json .sinan-dev/packages/app-search/manifest.json
  go build -o .sinan-dev/packages/app-search/backend/plugin ./plugins/system-app-search/cmd
  if [ ! -d .sinan-dev/plugins/system.app-search ]; then
    ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native install .sinan-dev/packages/app-search
  else
    ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native upgrade .sinan-dev/packages/app-search
  fi
fi

if [ ! -d .sinan-dev/plugins/system.settings ]; then
  ./bin/sinanctl -plugins .sinan-dev/plugins install plugins/system-settings
else
  ./bin/sinanctl -plugins .sinan-dev/plugins upgrade plugins/system-settings
fi

if [ ! -d .sinan-dev/plugins/system.plugin-manager ]; then
  ./bin/sinanctl -plugins .sinan-dev/plugins install plugins/system-plugin-manager
else
  ./bin/sinanctl -plugins .sinan-dev/plugins upgrade plugins/system-plugin-manager
fi

if [ ! -d .sinan-dev/plugins/system.plugin-store ]; then
  ./bin/sinanctl -plugins .sinan-dev/plugins install plugins/system-plugin-store
else
  ./bin/sinanctl -plugins .sinan-dev/plugins upgrade plugins/system-plugin-store
fi
