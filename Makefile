.PHONY: json-plugin setup bindings frontend build package run dev demo test ui-probe dev-ui ui-acceptance

# Node entry point also works directly from PowerShell without Make or sh.
setup bindings frontend build run package dev test:
	node scripts/desktop.mjs $@

# Compatibility alias: plugin UI is now enabled by default.
dev-ui: dev

# Compatibility alias; the development instance contains only system plugins.
demo: dev

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
