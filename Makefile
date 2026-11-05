.PHONY: setup bindings frontend build package run dev demo test ui-probe

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
	mkdir -p bin/Sinan.app/Contents/MacOS bin/Sinan.app/Contents/Resources
	cp bin/sinan bin/Sinan.app/Contents/MacOS/sinan.next
	mv -f bin/Sinan.app/Contents/MacOS/sinan.next bin/Sinan.app/Contents/MacOS/sinan
	cp apps/desktop/Info.plist bin/Sinan.app/Contents/Info.plist
	cp apps/desktop/assets/AppIcon.icns bin/Sinan.app/Contents/Resources/AppIcon.icns

dev: build
	sh scripts/prepare-dev.sh
	./bin/sinan -plugins .sinan-dev/plugins

# Compatibility alias; the development instance contains only system plugins.
demo: dev

test: frontend
	go test -race ./...

ui-probe:
	mkdir -p bin
	go build -o bin/sinan-ui-probe ./apps/ui-probe
	./bin/sinan-ui-probe
	./bin/sinan-ui-probe -flood
	./bin/sinan-ui-probe -revoke
	./bin/sinan-ui-probe -fault script
	./bin/sinan-ui-probe -fault hang
	./bin/sinan-ui-probe -fault slow
