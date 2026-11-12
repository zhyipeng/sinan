.PHONY: json-plugin setup bindings frontend build package run dev demo test ui-probe dev-ui ui-acceptance

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
	go build -o bin/sinan-ui-host ./apps/ui-host

run: build
	./bin/sinan

package: build
	test "$$(go env GOOS)" = darwin
	mkdir -p bin/Sinan.app/Contents/MacOS bin/Sinan.app/Contents/Resources
	cp bin/sinan bin/Sinan.app/Contents/MacOS/sinan.next
	mv -f bin/Sinan.app/Contents/MacOS/sinan.next bin/Sinan.app/Contents/MacOS/sinan
	cp bin/sinan-ui-host bin/Sinan.app/Contents/MacOS/sinan-ui-host.next
	mv -f bin/Sinan.app/Contents/MacOS/sinan-ui-host.next bin/Sinan.app/Contents/MacOS/sinan-ui-host
	cp apps/desktop/Info.plist bin/Sinan.app/Contents/Info.plist
	cp apps/desktop/assets/AppIcon.icns bin/Sinan.app/Contents/Resources/AppIcon.icns

dev: build
	sh scripts/prepare-dev.sh
	./bin/sinan -plugins .sinan-dev/plugins

# Compatibility alias: plugin UI is now enabled by default.
dev-ui: dev

# Compatibility alias; the development instance contains only system plugins.
demo: dev

test: frontend
	node --test packages/plugin-sdk/theme/runtime.test.mjs
	node --experimental-strip-types --test apps/desktop/frontend/tests/paste.test.mjs
	SINAN_NATIVE_UI_TEST=0 go test -race -p 1 ./...

ui-probe:
	mkdir -p bin
	go build -o bin/sinan-ui-probe ./apps/ui-probe
	./bin/sinan-ui-probe
	./bin/sinan-ui-probe -flood
	./bin/sinan-ui-probe -revoke
	./bin/sinan-ui-probe -fault script
	./bin/sinan-ui-probe -fault hang
	./bin/sinan-ui-probe -fault slow
	SINAN_NATIVE_UI_TEST=1 go test -race -count=1 ./internal/plugin/uiwindows -run TestNativeHost

# Disposable environment: never prepares or installs into .sinan-dev.
ui-acceptance: build
	./bin/sinan -ui-acceptance -acceptance-packages "$(CURDIR)/plugins"

# Optional compatibility entry; official plugins have an independent repository.
OFFICIAL_PLUGINS_DIR ?= ../sinan-official-plugins
json-plugin:
	@test -f "$(OFFICIAL_PLUGINS_DIR)/Makefile" || { echo "请先检出官方插件仓库，或设置 OFFICIAL_PLUGINS_DIR"; exit 1; }
	$(MAKE) -C "$(OFFICIAL_PLUGINS_DIR)" build
