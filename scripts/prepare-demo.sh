#!/bin/sh
set -eu
mkdir -p .sinan-dev/packages/echo/backend
cp plugins/example-echo/manifest.json .sinan-dev/packages/echo/manifest.json
go build -o .sinan-dev/packages/echo/backend/plugin ./plugins/example-echo/cmd
if [ ! -d .sinan-dev/plugins/example.hello ]; then
  ./bin/sinanctl -plugins .sinan-dev/plugins install plugins/example-hello
fi
if [ ! -d .sinan-dev/plugins/example.echo ]; then
  ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native install .sinan-dev/packages/echo
fi

for speed in fast slow; do
  mkdir -p ".sinan-dev/packages/search-$speed/backend"
  cp "plugins/example-search-$speed/manifest.json" ".sinan-dev/packages/search-$speed/manifest.json"
  go build -o ".sinan-dev/packages/search-$speed/backend/plugin" "./plugins/example-search-$speed/cmd"
  if [ ! -d ".sinan-dev/plugins/example.search-$speed" ]; then
    ./bin/sinanctl -plugins .sinan-dev/plugins -trust-native install ".sinan-dev/packages/search-$speed"
  fi
done
