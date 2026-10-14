# waybar-unifi-cam — build & install
#
# Quick start:
#   make build            # compile ./bin/waybar-unifi-cam
#   make install          # build, then install the binary and the systemd user unit
#   make enable-service   # import Wayland env + enable/start the daemon (opt-in)
#
# The Waybar and Hyprland snippets are NOT installed automatically because they
# must be merged into your existing config files by hand. See `make help`.

BINARY      := waybar-unifi-cam
PREFIX      ?= $(HOME)/.local
BINDIR      ?= $(PREFIX)/bin
SYSTEMD_DIR ?= $(HOME)/.config/systemd/user
UNIT        := waybar-unifi-cam.service

GO      ?= go
GOFLAGS ?=

.DEFAULT_GOAL := help

.PHONY: help build test vet race check install install-bin install-service \
        enable-service disable-service uninstall clean

help: ## Show this help
	@echo "waybar-unifi-cam targets:"
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*## "}{printf "  %-16s %s\n", $$1, $$2}'
	@echo
	@echo "Install dirs: BINDIR=$(BINDIR)  SYSTEMD_DIR=$(SYSTEMD_DIR)"
	@echo
	@echo "After 'make install', wire up your desktop by hand:"
	@echo "  - Waybar:   merge waybar/doorbell.jsonc into ~/.config/waybar/config.jsonc"
	@echo "              (add \"custom/doorbell\" to a modules list) and append"
	@echo "              waybar/style.css to ~/.config/waybar/style.css, then reload Waybar."
	@echo "  - Hyprland: append hyprland/windowrule.conf to your Hyprland config, then 'hyprctl reload'."
	@echo "  - Config:   create ~/.config/waybar-unifi-cam/config.json (chmod 600) and run"
	@echo "              '$(BINARY) trust' to pin the UDM Pro certificate. See README.md."

build: ## Compile the binary to ./bin/waybar-unifi-cam
	$(GO) build $(GOFLAGS) -o bin/$(BINARY) .

test: ## Run the unit test suite
	$(GO) test ./...

vet: ## Run go vet
	$(GO) vet ./...

race: ## Run the race detector on the concurrency-sensitive view package
	$(GO) test -race ./internal/view/

check: vet test race ## Run vet + tests + race

install: install-bin install-service ## Install the binary and the systemd user unit

install-bin: build ## Install the binary into BINDIR
	@mkdir -p "$(BINDIR)"
	install -m 0755 bin/$(BINARY) "$(BINDIR)/$(BINARY)"
	@echo "installed $(BINDIR)/$(BINARY)"

install-service: ## Install the systemd user unit into SYSTEMD_DIR
	@mkdir -p "$(SYSTEMD_DIR)"
	install -m 0644 systemd/$(UNIT) "$(SYSTEMD_DIR)/$(UNIT)"
	@systemctl --user daemon-reload 2>/dev/null || true
	@echo "installed $(SYSTEMD_DIR)/$(UNIT) (run 'make enable-service' to start it)"

enable-service: ## Make the Wayland env visible to user services, then enable+start the daemon
	systemctl --user import-environment WAYLAND_DISPLAY XDG_CURRENT_DESKTOP HYPRLAND_INSTANCE_SIGNATURE
	systemctl --user enable --now $(UNIT)
	@echo "daemon enabled; follow logs with: journalctl --user -u $(UNIT) -f"

disable-service: ## Stop and disable the daemon
	-systemctl --user disable --now $(UNIT)

uninstall: ## Remove the installed binary and systemd user unit
	-systemctl --user disable --now $(UNIT) 2>/dev/null
	rm -f "$(BINDIR)/$(BINARY)"
	rm -f "$(SYSTEMD_DIR)/$(UNIT)"
	@systemctl --user daemon-reload 2>/dev/null || true
	@echo "removed $(BINDIR)/$(BINARY) and $(SYSTEMD_DIR)/$(UNIT)"

clean: ## Remove build artifacts
	rm -rf bin/
