#!/bin/sh
set -eu
mkdir -p .sinan-dev/packages/echo/backend
cp plugins/example-echo/manifest.json .sinan-dev/packages/echo/manifest.json
go build -o .sinan-dev/packages/echo/backend/plugin ./plugins/example-echo/cmd
if [ ! -d .sinan-dev/plugins/example.hello ]; then
  ./bin/sinanctl -plugins .sinan-dev/plugins install plugins/example-hello
else
  ./bin/sinanctl -plugins .sinan-dev/plugins upgrade plugins/example-hello
fi
if [ ! -d .sinan-dev/plugins/example.echo ]; then
  ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native install .sinan-dev/packages/echo
else
  ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native upgrade .sinan-dev/packages/echo
fi

for speed in fast slow; do
  mkdir -p ".sinan-dev/packages/search-$speed/backend"
  cp "plugins/example-search-$speed/manifest.json" ".sinan-dev/packages/search-$speed/manifest.json"
  go build -o ".sinan-dev/packages/search-$speed/backend/plugin" "./plugins/example-search-$speed/cmd"
  if [ ! -d ".sinan-dev/plugins/example.search-$speed" ]; then
    ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native install ".sinan-dev/packages/search-$speed"
  else
    ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native upgrade ".sinan-dev/packages/search-$speed"
  fi
done

# First practical system plugin; installed after the existing examples.
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
