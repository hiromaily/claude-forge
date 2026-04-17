---
name: forge-setup
description: Configure per-repository default flags for /forge pipelines. Interactive setup that saves preferences to .specs/preferences.json.
---

# forge-setup

Configure default flags for `/forge` pipelines in this repository.

## Flow

1. Call `mcp__forge-state__preferences_get` to load current preferences.

2. Present all current values and ask the user to confirm or change them using AskUserQuestion. Ask all five questions in a single prompt:

   ```
   Current preferences:
   - --auto (skip human confirmation): {current or "not set"}
   - --debug (debug mode): {current or "not set"}
   - --effort (default effort level): {current or "not set (auto-detect)"}
   - --nopr (skip PR creation): {current or "not set"}
   - --discuss (pre-pipeline discussion): {current or "not set"}

   For each setting, reply with the desired value:
   1. auto: yes / no / keep
   2. debug: yes / no / keep
   3. effort: S / M / L / none / keep
   4. nopr: yes / no / keep
   5. discuss: yes / no / keep
   ```

3. Parse the user's responses and build the preferences object.

4. Show a summary and ask for final confirmation:

   ```
   Preferences to save:
   - auto: true
   - debug: false
   - effort: M
   - nopr: true
   - discuss: false

   Save these preferences? (yes/no)
   ```

5. If confirmed, call `mcp__forge-state__preferences_set` with the preferences object.

6. Display confirmation: "Preferences saved to `.specs/preferences.json`. These defaults will be applied to all future `/forge` runs. Explicit flags on `/forge` always override preferences."

## Notes

- "keep" means retain the current value (or "not set" if there is no current value).
- "none" for effort means remove the preference (auto-detect effort).
- This skill only reads/writes through MCP tools; it does not manipulate files directly.
- Preferences can only enable flags. Setting a flag to "no" removes the preference (same as "not set").
