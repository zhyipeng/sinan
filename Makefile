.PHONY: setup bindings frontend build package run demo test

ifeq ($(shell go env GOOS),darwin)
export CGO_CFLAGS += -mmacosx-version-min=12.0
export CGO_LDFLAGS += -mmacosx-version-min=12.0
export MACOSX_DEPLOYMENT_TARGET = 12.0
endif

setup:
	go mod download
	npm --prefix apps/desktop/frontend ci

bindings:
	mkdir -p apps/desktop/frontend/dist
	touch apps/desktop/frontend/dist/.placeholder
	wails3 generate bindings -ts -i -noevents -d apps/desktop/frontend/bindings ./apps/desktop

frontend: bindings
	npm --prefix apps/desktop/frontend run build

build: frontend
	mkdir -p bin
	go build -o bin/sinan ./apps/desktop
	go build -o bin/sinanctl ./apps/cli

run: build
	./bin/sinan

package: build
	test "$$(go env GOOS)" = darwin
	mkdir -p bin/Sinan.app/Contents/MacOS
	cp bin/sinan bin/Sinan.app/Contents/MacOS/sinan.next
	mv -f bin/Sinan.app/Contents/MacOS/sinan.next bin/Sinan.app/Contents/MacOS/sinan
	cp apps/desktop/Info.plist bin/Sinan.app/Contents/Info.plist

demo: build
	sh scripts/prepare-demo.sh
	./bin/sinan -plugins .sinan-dev/plugins

test: frontend
	go test -race ./...
