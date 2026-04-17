> **調査日:** 2026-04-16
> **問い:** claude-forge を、現行の Claude Code プラグインと等価な OpenAI Codex プラグインとして配布できるか？

## 背景

claude-forge は以下のアーティファクトを Claude Code プラグイン1パッケージとして配布しています:

- `.claude-plugin/plugin.json` + `marketplace.json`
- `.mcp.json`（forge-state MCP サーバ登録）
- `agents/*.md` — 10 個の named サブエージェント定義
- `skills/forge/SKILL.md` — オーケストレータ skill
- `hooks/hooks.json` — `PreToolUse` / `PostToolUse` / `Stop` / `Setup`
- `scripts/*.sh` — `${CLAUDE_PLUGIN_ROOT}` を参照する hook 実装
- `mcp-server/` — Go バイナリ、46 ツール

本ドキュメントは、2026 年 4 月時点で同じバンドルを Codex プラグインとして配布できるかを評価します。

## 主要な発見

Codex には plugin 機構が実装済み（`codex marketplace add`、v0.121.0）。プラグインバンドルが公式に対応しているのは `skills` / `mcpServers` / `apps` のみで、**`agents` および `hooks` フィールドは存在しません**。Open issue による2つのブロッカーにより、claude-forge のコア設計を忠実に移植することはできません。

## 比較マトリクス

| claude-forge 要素 | Claude Code | Codex (2026-04) | 等価性 | 備考 |
|---|---|---|---|---|
| プラグイン配布 | `.claude-plugin/plugin.json` + marketplace | `.codex-plugin/plugin.json`、`codex marketplace add` (v0.121.0) | ◎ | GitHub / git / ローカル / URL から install 可 |
| バンドル可能要素 | MCP + agents + hooks + skills + commands + scripts | `skills` + `mcpServers` + `apps` のみ（公式スキーマ） | △ | `agents` / `hooks` 同梱は非対応 |
| MCP サーバ | `.mcp.json`、stdio + http | `[mcp_servers.<name>]` TOML、`codex mcp add`、stdio + streamable HTTP | ◎ | `env`、`bearer_token_env_var`、OAuth 対応 |
| サブエージェント定義 | `agents/*.md`（frontmatter） | `~/.codex/agents/*.toml` または `.codex/agents/*.toml` | ○ | フィールド: `name` / `description` / `developer_instructions` / `model` / `model_reasoning_effort` / `sandbox_mode` |
| **オーケストレータからの subagent 起動** | `Agent` / `Task` ツールで任意の名前指定 | `spawn_agent` は **tool-backed セッションから named custom subagent を起動できない**（[#15250](https://github.com/openai/codex/issues/15250)、open） | ✕ | **ブロッカー #1 — コア設計が破綻** |
| 隔離コンテキスト / 並列実行 | `Agent` 呼び出し毎に独立 | `agents.max_threads=6`, `max_depth=1`（デフォルト） | ○ | v0.119 で background progress streaming 追加 |
| サブエージェントのプラグイン配布 | プラグイン内 `agents/` | **plugin.json スキーマに `agents` フィールドなし** | ✕ | ユーザが手動で `~/.codex/agents/` に TOML を配置する必要あり |
| Skills / slash | `skills/*/SKILL.md` | 同パターン、`/skills` または `$skill` で起動 | ◎ | プラグイン同梱可 |
| カスタムプロンプト | `commands/*.md` | `custom-prompts` は deprecated → skills へ移行推奨 | ○ | |
| Hook イベント | PreToolUse / PostToolUse / Stop / Setup / UserPromptSubmit | 同名イベント | ○ | 同 stdin JSON、`exit 2 = block` |
| **Hook 発火対象ツール** | 全ツール | **PreToolUse / PostToolUse は `shell` (Bash) のみ発火**（[#14754](https://github.com/openai/codex/issues/14754), [#16732](https://github.com/openai/codex/issues/16732)） | ✕ | **ブロッカー #2 — Phase 1/2 の Write/Edit ガードが機能不全** |
| Hook のプラグイン配布 | プラグイン内 `hooks/hooks.json` | plugin.json スキーマに `hooks` フィールドなし | △ | プラグインレイヤでの hook 同梱は非公式 |
| `${CLAUDE_PLUGIN_ROOT}` 相当 | ホストが注入する環境変数 | Codex では未文書化 | ✕ | hook スクリプトからプラグインインストールディレクトリを参照できない |
| プロジェクトメモリ | `CLAUDE.md`（project + user 階層） | `AGENTS.md`（自動ロード、project + user 階層） | ○ | プラグインから project-level 指示を追加する公式 API はない |
| ツール名 | `Agent` / `Bash` / `Edit` / `Write` / `Read` / `Glob` / `Grep` / `TodoWrite` | `shell` / `apply_patch` / `spawn_agent` / `spawn_agents_on_csv`（Read/Glob/Grep/TodoWrite 直接対応なし） | △ | エージェント prompt の書き換え必須 |

**凡例:** ◎ 同等 / ○ 代替可 / △ 部分的 / ✕ なし

## 致命的ブロッカー

### ブロッカー #1 — Tool-backed セッションから named subagent を起動できない

[openai/codex#15250](https://github.com/openai/codex/issues/15250)（2026-04 時点で open）。`.codex/agents/*.toml` のカスタムエージェントは自然言語 CLI からは起動できますが、skill オーケストレータが使う `spawn_agent` ツールは generic な agent type しか受け付けません。claude-forge は「オーケストレータ skill が各フェーズで特定の named サブエージェントを spawn する（パイプライン全体で 10 種の専門エージェントを使い分ける）」設計が前提で、**現在の tool API ではこのパターンを表現できません**。

コミュニティ回避策（例: [leonardsellem/codex-subagents-mcp](https://github.com/leonardsellem/codex-subagents-mcp)）は、TOML を読んで `developer_instructions` を generic worker に注入する外部 MCP を立てるパターンですが、これではネイティブな subagent 隔離セマンティクスが失われます。

### ブロッカー #2 — Write / Edit / apply_patch で hook が発火しない

[openai/codex#14754](https://github.com/openai/codex/issues/14754) および [openai/codex#16732](https://github.com/openai/codex/issues/16732) によれば、`PreToolUse` / `PostToolUse` イベントは `shell` ツールに対してのみ発火します。claude-forge の `pre-tool-hook.sh` は Phase 1/2 の read-only モード（situation-analyst / investigator 中の `Edit` / `Write` ブロック）を強制しますが、**Codex には現状この強制ポイントが存在しません**。

## 互換性のある領域

以下は摩擦少なく移植できます:

- **MCP サーバ** — Go バイナリ（`forge-state`、46 ツール）はそのまま動作。登録形式のみ変更（`.codex-plugin/plugin.json` の `mcpServers` エントリまたは `~/.codex/config.toml` の `[mcp_servers.*]`）
- **`SKILL.md`** — Codex skill のスキーマは Claude Code とほぼ一致。`skills/forge/SKILL.md` は最小限の編集で再利用可
- **Hook payload / exit code** — stdin JSON 規約と `exit 2 = block` は同一。`pre-tool-hook.sh` のうち `Bash` ツールを対象とするルール（Phase 5 並列実行中の `git commit` ブロック、`main`/`master` への `git checkout` ブロック）は Codex の `shell` でも発火する。沈黙するのは `Edit` / `Write` 系ルールのみ
- **AGENTS.md** — Codex ネイティブ規約。`CLAUDE.md` から派生可

## 結論

**2026 年 4 月時点で、claude-forge を Codex プラグインとして配布することは推奨しない**。upstream のブロッカー解決を待つか、忠実度を落とした移植を受け入れる二択。

判断を覆す upstream マイルストン:

1. [openai/codex#15250](https://github.com/openai/codex/issues/15250) の解決 — `spawn_agent` が named custom agents を受け付ける
2. `.codex-plugin/plugin.json` スキーマに `agents` / `hooks` フィールドが追加される
3. hook スクリプト用の `CODEX_PLUGIN_ROOT`（または相当の環境変数）が文書化される
4. `PreToolUse` / `PostToolUse` の発火対象が `apply_patch`（Write / Edit 相当）に拡張される

それまで部分移植（忠実度 60-70%）で進める場合: `forge-state` MCP + `SKILL.md` のみ配布、subagent TOML はユーザが手動配置、Phase 1/2 の write-guard は諦める。この状態のパイプラインは「サブエージェント隔離による context contamination 防止」という claude-forge の核心的な価値提案を提供できなくなります。

## 参考資料

- [Codex Plugins — Build guide](https://developers.openai.com/codex/plugins/build)
- [Codex Subagents](https://developers.openai.com/codex/subagents)
- [Codex Skills](https://developers.openai.com/codex/skills)
- [Codex Hooks](https://developers.openai.com/codex/hooks)
- [Codex MCP](https://developers.openai.com/codex/mcp)
- [Codex Changelog](https://developers.openai.com/codex/changelog)
- [Codex AGENTS.md ガイド](https://developers.openai.com/codex/guides/agents-md)
- [openai/codex#15250 — named subagents not accessible from tool-backed sessions](https://github.com/openai/codex/issues/15250)
- [openai/codex#14754 — PreToolUse / PostToolUse coverage for non-Bash tools](https://github.com/openai/codex/issues/14754)
- [openai/codex#16732 — ApplyPatchHandler hook events](https://github.com/openai/codex/issues/16732)
- [leonardsellem/codex-subagents-mcp — 回避策パターン](https://github.com/leonardsellem/codex-subagents-mcp)

## 注意事項

上記の発見は 2026-04-16 時点の Web 調査に基づくもので、Codex 0.121.0 をローカルで実行して独立検証はしていません。本ドキュメントを根拠に意思決定する前に:

1. [openai/codex#15250](https://github.com/openai/codex/issues/15250) の状態を再確認すること（最重要ブロッカー）
2. `.codex-plugin/plugin.json` の最新スキーマで `agents` / `hooks` フィールドが追加されていないか再確認すること
3. `apply_patch`（Write / Edit 相当）で hook が発火するようになっていないか再確認すること
4. `CODEX_PLUGIN_ROOT` 相当の環境変数が文書化されたか確認すること

最新の Codex 公式ドキュメントと本ドキュメントが食い違う場合は公式を優先し、本ドキュメントを更新してください。
