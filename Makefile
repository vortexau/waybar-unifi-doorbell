# waybar-unifi-cam — build, install & desktop wiring
#
# Quick start:
#   make build            # compile ./bin/waybar-unifi-cam
#   make install          # install the binary and the systemd user unit
#   make setup            # install + scaffold config + wire Waybar style + Hyprland rule
#   make trust            # pin the UDM Pro certificate (after editing config.json)
#   make enable-service   # import Wayland env + enable/start the daemon (opt-in)
#
# All the wiring targets are idempotent: re-running them does nothing if the
# change is already present. The ONE step that cannot be automated is adding the
# "custom/doorbell" definition block to your Waybar config.jsonc — that is a
# structured edit of a hand-commented file; `make check-waybar-config` tells you
# whether it still needs doing. See README.md "Wire up Waybar & Hyprland".

BINARY        := waybar-unifi-cam
PREFIX        ?= $(HOME)/.local
BINDIR        ?= $(PREFIX)/bin
SYSTEMD_DIR   ?= $(HOME)/.config/systemd/user
UNIT          := waybar-unifi-cam.service

CONFIG_DIR    ?= $(HOME)/.config/waybar-unifi-cam
CONFIG_FILE   ?= $(CONFIG_DIR)/config.json
HOST          ?= 192.168.1.1

WAYBAR_DIR    ?= $(HOME)/.config/waybar
WAYBAR_STYLE  ?= $(WAYBAR_DIR)/style.css
WAYBAR_CONFIG ?= $(WAYBAR_DIR)/config.jsonc

HYPR_DIR      ?= $(HOME)/.config/hypr
HYPR_CONFIG   ?= $(HYPR_DIR)/hyprland.conf
HYPR_RULE     := $(HYPR_DIR)/waybar-unifi-cam.conf

GO      ?= go
GOFLAGS ?=

define CONFIG_TEMPLATE
{
  "host": "$(HOST)",
  "api_key": "PASTE_YOUR_API_KEY_HERE",
  "camera": "",
  "window": { "width": 960, "height": 540, "position": "center", "always_on_top": true },
  "ring_timeout_seconds": 30,
  "audio": true
}
endef
export CONFIG_TEMPLATE

.DEFAULT_GOAL := help

.PHONY: help build test vet race check install install-bin install-service \
        setup install-config install-waybar-style install-hypr \
        check-waybar-config trust reload enable-service disable-service \
        uninstall clean

help: ## Show this help
	@echo "waybar-unifi-cam targets:"
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*## "}{printf "  %-20s %s\n", $$1, $$2}'
	@echo
	@echo "Paths: BINDIR=$(BINDIR)  CONFIG=$(CONFIG_FILE)"
	@echo "       WAYBAR_STYLE=$(WAYBAR_STYLE)  HYPR_CONFIG=$(HYPR_CONFIG)"
	@echo
	@echo "'make setup' does everything it safely can. The one manual step left"
	@echo "is adding the \"custom/doorbell\" block to $(WAYBAR_CONFIG)"
	@echo "and listing \"custom/doorbell\" in a modules-* array (run"
	@echo "'make check-waybar-config' to see if it is still needed)."

# ── Build ────────────────────────────────────────────────────────────────

build: ## Compile the binary to ./bin/waybar-unifi-cam
	$(GO) build $(GOFLAGS) -o bin/$(BINARY) .

test: ## Run the unit test suite
	$(GO) test ./...

vet: ## Run go vet
	$(GO) vet ./...

race: ## Run the race detector on the view package
	$(GO) test -race ./internal/view/

check: vet test race ## Run vet + tests + race

# ── Install (binary + service) ───────────────────────────────────────────

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

# ── Full desktop wiring ──────────────────────────────────────────────────

setup: install install-config install-waybar-style install-hypr check-waybar-config ## Install + scaffold config + wire Waybar style & Hyprland rule
	@echo
	@echo "Next:"
	@echo "  1. Edit $(CONFIG_FILE) and paste your Protect API key."
	@echo "  2. make trust            # pin the UDM Pro cert (confirm the fingerprint)"
	@echo "  3. Add the Waybar module block if the check above says it is missing."
	@echo "  4. make reload           # reload Waybar, then: hyprctl reload"
	@echo "  5. make enable-service   # (optional) run the daemon via systemd"

install-config: ## Scaffold ~/.config/waybar-unifi-cam/config.json (0600, never clobbers)
	@mkdir -p "$(CONFIG_DIR)"
	@if [ -f "$(CONFIG_FILE)" ]; then \
		echo "config exists, leaving it untouched: $(CONFIG_FILE)"; \
	else \
		printf '%s\n' "$$CONFIG_TEMPLATE" > "$(CONFIG_FILE)"; \
		chmod 600 "$(CONFIG_FILE)"; \
		echo "wrote $(CONFIG_FILE) (0600) — edit it and paste your API key"; \
	fi

install-waybar-style: ## Append the #custom-doorbell CSS rule to your Waybar style.css (idempotent)
	@if [ ! -f "$(WAYBAR_STYLE)" ]; then \
		echo "no $(WAYBAR_STYLE); skipping (set WAYBAR_STYLE=... if yours is elsewhere)"; \
	elif grep -q 'custom-doorbell' "$(WAYBAR_STYLE)"; then \
		echo "#custom-doorbell rule already present in $(WAYBAR_STYLE); skipping"; \
	else \
		printf '\n' >> "$(WAYBAR_STYLE)"; \
		cat waybar/style.css >> "$(WAYBAR_STYLE)"; \
		echo "appended #custom-doorbell rule to $(WAYBAR_STYLE)"; \
	fi

install-hypr: ## Install the Hyprland window rule (hyprlang configs only; Lua is manual)
	@if [ -f "$(HYPR_DIR)/hyprland.lua" ]; then \
		echo "Lua Hyprland config detected ($(HYPR_DIR)/hyprland.lua)."; \
		echo "This target only wires a traditional hyprland.conf. Paste the block"; \
		echo "from hyprland/windowrule.lua into your hyprland.lua by hand, then"; \
		echo "'hyprctl reload'. The block to add:"; \
		echo ""; \
		sed 's/^/    /' hyprland/windowrule.lua; \
	else \
		mkdir -p "$(HYPR_DIR)"; \
		install -m 0644 hyprland/windowrule.conf "$(HYPR_RULE)"; \
		echo "installed $(HYPR_RULE)"; \
		if [ ! -f "$(HYPR_CONFIG)" ]; then \
			echo "no $(HYPR_CONFIG); add this line to your Hyprland config:"; \
			echo "    source = $(HYPR_RULE)"; \
		elif grep -q 'waybar-unifi-cam.conf' "$(HYPR_CONFIG)"; then \
			echo "source line already present in $(HYPR_CONFIG); skipping"; \
		else \
			printf '\nsource = %s\n' "$(HYPR_RULE)" >> "$(HYPR_CONFIG)"; \
			echo "added 'source = $(HYPR_RULE)' to $(HYPR_CONFIG)"; \
		fi; \
	fi

check-waybar-config: ## Report whether the custom/doorbell module is wired in config.jsonc
	@if [ ! -f "$(WAYBAR_CONFIG)" ]; then \
		echo "no $(WAYBAR_CONFIG) found (set WAYBAR_CONFIG=...)"; \
	elif grep -Eq '"custom/doorbell"[[:space:]]*:' "$(WAYBAR_CONFIG)"; then \
		echo "OK: \"custom/doorbell\" definition found in $(WAYBAR_CONFIG)"; \
	else \
		echo "ACTION NEEDED: add the \"custom/doorbell\" block to $(WAYBAR_CONFIG)"; \
		echo "  (and list \"custom/doorbell\" in a modules-* array). This cannot be"; \
		echo "  auto-edited without reformatting your commented config. The block to add:"; \
		echo ""; \
		sed 's/^/    /' waybar/doorbell.jsonc; \
	fi

trust: ## Pin the UDM Pro certificate (runs the installed binary's 'trust')
	"$(BINDIR)/$(BINARY)" trust

reload: ## Reload Waybar (SIGUSR2)
	-killall -SIGUSR2 waybar
	@echo "reloaded Waybar (for Hyprland run: hyprctl reload)"

# ── Service control ──────────────────────────────────────────────────────

enable-service: ## Import Wayland env, then enable+start the daemon
	systemctl --user import-environment WAYLAND_DISPLAY XDG_CURRENT_DESKTOP HYPRLAND_INSTANCE_SIGNATURE
	systemctl --user enable --now $(UNIT)
	@echo "daemon enabled; follow logs with: journalctl --user -u $(UNIT) -f"

disable-service: ## Stop and disable the daemon
	-systemctl --user disable --now $(UNIT)

# ── Teardown ─────────────────────────────────────────────────────────────

uninstall: ## Remove the installed binary and systemd user unit
	-systemctl --user disable --now $(UNIT) 2>/dev/null
	rm -f "$(BINDIR)/$(BINARY)"
	rm -f "$(SYSTEMD_DIR)/$(UNIT)"
	@systemctl --user daemon-reload 2>/dev/null || true
	@echo "removed $(BINDIR)/$(BINARY) and $(SYSTEMD_DIR)/$(UNIT)"
	@echo "note: left your config, Waybar style, and Hyprland edits in place"

clean: ## Remove build artifacts
	rm -rf bin/
