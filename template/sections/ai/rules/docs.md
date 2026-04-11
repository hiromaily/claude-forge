---
paths: ["docs/**/*.md", "template/**/*.md", "README.md", "CLAUDE.md"]
---

# Documentation Rules

## Generated Files (docs-ssot)

`README.md` and `CLAUDE.md` are **generated files** — do not edit them directly.

- Edit source templates under `template/` and run `make docs` to regenerate.
- Templates live at `template/pages/README.tpl.md` and `template/pages/CLAUDE.tpl.md`.
- Section files live under `template/sections/`.
- Configuration: `docsgen.yaml`

### Critical rules

- **Never edit `README.md` or `CLAUDE.md` directly** — changes are overwritten on next `make docs`.
- To change structure, edit the template (`template/*.tpl.md`).
- To change section content, edit the relevant file under `template/sections/`.
- For sections that include from `docs/_partials/`, edit the partial — not the section file.

### Workflow

```sh
make docs           # regenerate README.md and CLAUDE.md
make docs-validate  # dry-run: check all includes resolve
make docs-check     # detect near-duplicate sections
make docs-index     # print include relationship graph
```

---

## Bilingual Documentation

The `docs/` directory contains documentation in two languages:

- English: `docs/` (root-level markdown files and subdirectories)
- Japanese: `docs/ja/` (mirrors the English structure)

### Rules

- When adding or modifying any documentation under `docs/`, always update the English version first, then update the corresponding Japanese translation under `docs/ja/`.
- Both language versions must be kept in sync; never update one without updating the other.
- Japanese files mirror the English directory structure (e.g., `docs/architecture/foo.md` → `docs/ja/architecture/foo.md`).
- This project follows SSOT principles. When editing documentation, always identify and update the SSOT location first.

### Exceptions

The following directories are **excluded** from the bilingual requirement — English only, no `docs/ja/` counterpart needed:

- `docs/ai-friendly-audit-report/` — auto-generated audit reports

## SSOT via Modularized Partials

The `docs/_partials/` directory holds reusable content fragments that are the **single source of truth** for content shared across multiple documents.

### Rules

- When content appears in more than one document, extract it into a partial under `docs/_partials/` rather than duplicating it.
- Each partial file must begin with an HTML comment that declares its purpose and lists every file that includes it, e.g.:
  ```
  <!-- SSOT: <description>.
       Included by:
         docs/foo.md,
         docs/bar/baz.md
       Edit only this file when <content> changes. -->
  ```
- When editing shared content, **always edit the partial**, not the including files.
- When adding a new inclusion, update the partial's header comment to list the new consumer.
- Partials under `docs/_partials/` follow the same bilingual rule: if the content has a Japanese equivalent, create a corresponding `docs/_partials/<name>-ja.md` partial.

## VitePress Site

The `docs/` directory is published as a GitHub Pages site using [VitePress](https://vitepress.dev/). The VitePress configuration is at `docs/.vitepress/config.ts`.

### Rules

- When adding a new document, add a corresponding link entry in `docs/.vitepress/config.ts` in the appropriate sidebar section and in the correct order.
- For bilingual docs, update both the English and Japanese nav/sidebar entries in the config.
- Never create a new document without making it reachable via the site navigation.
