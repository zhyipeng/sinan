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
