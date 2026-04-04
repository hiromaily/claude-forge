#------------------------------------------------------------------------------
# MCP Server Setup
# - Development for mcp-server is at mcp-server/Makefile
#------------------------------------------------------------------------------

MCP_DIR    := mcp-server
MCP_BINARY := forge-state-mcp
INSTALL_DIR := $(or $(GOBIN),$(HOME)/.local/bin)
APP_VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)

# build: Compile the MCP server binary to bin/forge-state-mcp
.PHONY: build
build:
	mkdir -p bin
	cd $(MCP_DIR)/cmd && go build -ldflags="-s -w -X main.appVersion=$(APP_VERSION)" -o ../../bin/$(MCP_BINARY) .
	@echo "$(APP_VERSION)" | sed 's/^v//' > bin/.installed-version

# install: Build and copy the binary to $(GOBIN) or ~/.local/bin
.PHONY: install
install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/$(MCP_BINARY) $(INSTALL_DIR)/$(MCP_BINARY)

# setup: Build and install the binary locally (for development without plugin install)
.PHONY: setup
setup: install
	@echo ""
	@echo "✓ Binary installed at $(INSTALL_DIR)/$(MCP_BINARY)"
	@echo ""
	@echo "For plugin users: the MCP server is auto-registered via .mcp.json."
	@echo "For local dev:    run 'make setup-manual' to register via claude mcp add."

# setup-manual: Register the MCP server manually with Claude Code (for local dev without plugin)
# Uses --scope local so this project-level override takes precedence over .mcp.json,
# avoiding a duplicate "forge-state" entry when working inside the claude-forge dev repo.
.PHONY: setup-manual
setup-manual: install
	@echo "Registering forge-state MCP server with Claude Code..."
	@claude mcp remove forge-state -s local 2>/dev/null || true
	claude mcp add forge-state \
		--transport stdio \
		--scope local \
		--env FORGE_AGENTS_PATH=$(CURDIR)/agents \
		-- $(INSTALL_DIR)/$(MCP_BINARY)
	@echo ""
	@echo "✓ Setup complete."
	@echo "  Binary:     $(INSTALL_DIR)/$(MCP_BINARY)"
	@echo "  Agents:     $(CURDIR)/agents"
	@echo ""
	@echo "⚠ Restart your Claude Code session to activate the MCP server."
	@echo "  After restart, run /mcp to verify forge-state shows as Connected."

# test: Run the Go test suite for the MCP server
.PHONY: test
test:
	cd $(MCP_DIR) && go test -race ./...

# clean: Remove the built binary
.PHONY: clean
clean:
	rm -f bin/$(MCP_BINARY)

#------------------------------------------------------------------------------
# Docs
#------------------------------------------------------------------------------
.PHONY: install-docs
install-docs:
	bun install


#------------------------------------------------------------------------------
# Release
#------------------------------------------------------------------------------

# update-tag: Update the version tag in marketplace.json and plugin metadata to new version
# e.g. make update-tag new=2.1.1 old=2.1.0
.PHONY: update-tag
update-tag:
	@echo "Updating tag to v${new} in marketplace.json"
	@sed -i '' 's/"version": "${old}"/"version": "${new}"/g' .claude-plugin/marketplace.json
	@echo "Tag updated to v${new} in marketplace.json"
	@echo "Updating tag to v${new} in plugin.json"
	@sed -i '' 's/"version": "${old}"/"version": "${new}"/g' .claude-plugin/plugin.json
	@echo "Tag updated to v${new} in plugin.json"

# update-git-tag: Create and push a git tag for the new version
# e.g. make update-git-tag new=2.1.0
.PHONY: update-git-tag
update-git-tag:
	@echo "Creating git tag v${new}"
	@git tag -a "v${new}" -m "Release version ${new}"
	@echo "Git tag v${new} created"
	@echo "Pushing git tag v${new} to origin"
	@git push origin "v${new}"
	@echo "Git tag v${new} pushed to origin"

# update-all: Update version, commit, tag, and push — full release flow
# e.g. make update-all new=2.1.1 old=2.1.0
.PHONY: update-all
update-all: update-tag
	@git add .claude-plugin/marketplace.json .claude-plugin/plugin.json
	@git commit -m "chore: bump version to v${new}"
	@git push origin main
	@$(MAKE) update-git-tag new=${new}
	@echo "Version updated to v${new}, committed, tagged, and pushed"

# e.g. make retag TAG=v2.1.0
.PHONY: retag
retag:
	git tag -d $(TAG) 2>/dev/null || true
	git push --delete origin $(TAG) 2>/dev/null || true
	git tag -a $(TAG) $(COMMIT) -m "retag $(TAG)"
	git push origin $(TAG)
