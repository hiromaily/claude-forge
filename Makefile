
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
