## Releasing a new version

When cutting a new release:

1. Update the version in `plugin.json`:
   ```bash
   make update-tag new=1.5.0 old=1.4.0
   ```

2. Commit and push:
   ```bash
   git add -A && git commit -m "chore: bump version to 1.5.0"
   git push origin main
   ```

3. Create and push the tag:
   ```bash
   make update-git-tag new=1.5.0
   ```

4. GitHub Actions (`release.yml`) automatically:
   - Cross-compiles `forge-state-mcp` for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64
   - Creates a GitHub Release with the gzipped binaries attached
   - Generates release notes from commits

When users update the plugin, the Setup hook re-runs and downloads the new binary.
