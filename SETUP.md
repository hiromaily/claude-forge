# claude-forge setup

Start a new Claude Code session in the terminal and enter the following commands:

```
# register for marketplaces
/plugin marketplace add hiromaily/claude-forge

# install
/plugin install claude-forge
 or
clone https://github.com/hiromaily/claude-forge.git
claude plugins install ~/<path>/hiromaily/claude-forge

# install with onetime session only
claude --plugin-dir ~/<path>/hiromaily/claude-forge

# update
claude plugin update claude-forge@claude-forge

# reload
/reload-plugins

# uninstall
claude plugins uninstall claude-forge@claude-forge
```
