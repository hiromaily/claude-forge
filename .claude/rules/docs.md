---
paths: ["docs/**/*.md"]
---

# Documentation Rules

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
