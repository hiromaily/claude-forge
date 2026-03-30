
MCP_DIR    := mcp-server
MCP_BINARY := forge-state-mcp
INSTALL_DIR := $(or $(GOBIN),$(HOME)/.local/bin)

# build: Compile the MCP server binary to bin/forge-state-mcp
.PHONY: build
build:
	mkdir -p bin
	cd $(MCP_DIR) && go build -o ../bin/$(MCP_BINARY) .

# install: Build and copy the binary to $(GOBIN) or ~/.local/bin
.PHONY: install
install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/$(MCP_BINARY) $(INSTALL_DIR)/$(MCP_BINARY)

# setup: Build, install, and register the MCP server with Claude Code (idempotent)
.PHONY: setup
setup: install
	@echo "Registering forge-state MCP server with Claude Code..."
	@claude mcp remove forge-state -s user 2>/dev/null || true
	claude mcp add forge-state \
		--transport stdio \
		--scope user \
		--env FORGE_AGENTS_PATH=$(CURDIR)/agents \
		-- $(INSTALL_DIR)/$(MCP_BINARY)
	@echo ""
	@echo "✓ Setup complete."
	@echo "  Binary:     $(INSTALL_DIR)/$(MCP_BINARY)"
	@echo "  Agents:     $(CURDIR)/agents"
	@echo ""
	@echo "⚠ Restart your Claude Code session to activate the MCP server."
	@echo "  After restart, run /mcp to verify forge-state shows as Connected."

# test: Run the Go test suite for mcp-server/
.PHONY: test
test:
	$(MAKE) -C mcp-server go-test

# clean: Remove the built binary
.PHONY: clean
clean:
	rm -f bin/$(MCP_BINARY)

# update-tag: Update the version tag in marketplace.json and plugin metadata to new version
# e.g. make update-tag new=1.1.0 old=1.0.0
.PHONY: update-tag
update-tag:
	@echo "Updating tag to v${new} in marketplace.json"
	@sed -i '' 's/"version": "${old}"/"version": "${new}"/g' .claude-plugin/marketplace.json
	@echo "Tag updated to v${new} in marketplace.json"
	@echo "Updating tag to v${new} in plugin.json"
	@sed -i '' 's/"version": "${old}"/"version": "${new}"/g' .claude-plugin/plugin.json
	@echo "Tag updated to v${new} in plugin.json"

# update-git-tag: Create and push a git tag for the new version
# e.g. make update-git-tag new=1.1.0
.PHONY: update-git-tag
update-git-tag:
	@echo "Creating git tag v${new}"
	@git tag -a "v${new}" -m "Release version ${new}"
	@echo "Git tag v${new} created"
	@echo "Pushing git tag v${new} to origin"
	@git push origin "v${new}"
	@echo "Git tag v${new} pushed to origin"

# update-all: Update the version tag in marketplace.json and plugin metadata, then create and push a git tag for the new version
# e.g. make update-all new=1.1.0 old=1.0.0
.PHONY: update-all
update-all: update-tag update-git-tag
	@echo "Version updated to v${new} and git tag created and pushed"

# e.g. make retag TAG=v1.1.0
.PHONY: retag
retag:
	git tag -d $(TAG) 2>/dev/null || true
	git push --delete origin $(TAG) 2>/dev/null || true
	git tag -a $(TAG) $(COMMIT) -m "retag $(TAG)"
	git push origin $(TAG)
