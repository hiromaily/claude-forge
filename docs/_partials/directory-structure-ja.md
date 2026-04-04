<!-- SSOT: claude-forgeリポジトリのディレクトリ構造（日本語版）。
     Included by:
       docs/ja/architecture/overview.md
     このファイルのみを編集してください。構造変更時は英語版 docs/_partials/directory-structure.md も更新してください。

     Note for Claude Code: このファイルは VitePress の <!--@include:--> ディレクティブで
     docs/ ファイルに組み込まれます。Claude Code はそのディレクティブを辿れないため、
     このファイルを直接読んでください。 -->

```
claude-forge/
├── CLAUDE.md              ← AIエージェントガイド（Claude Code が自動読み込み）
├── ARCHITECTURE.md        ← インデックス（詳細は docs/architecture/ を参照）
├── BACKLOG.md             ← 既知の問題・改善候補
├── README.md              ← プロジェクト概要とクイックスタート
├── .claude-plugin/
│   └── plugin.json        ← プラグインメタデータ（名前、バージョン）
├── .claude/
│   └── rules/             ← git.md、shell-script.md、docs.md
├── agents/                ← 10 の専門エージェント定義（.md ファイル）
├── docs/
│   ├── _partials/         ← SSOT コンテンツ断片（docs/ ファイルが組み込む）
│   └── architecture/      ← アーキテクチャドキュメント（13 ファイル）
├── hooks/
│   └── hooks.json         ← フック定義（PreToolUse、PostToolUse、Stop）
├── mcp-server/            ← Go MCPサーバーソース（forge-state バイナリ）
├── scripts/               ← フックスクリプト + テストスイート
│   ├── pre-tool-hook.sh   ← 読み取り専用ガード、コミットブロック、チェックアウトブロック
│   ├── post-agent-hook.sh ← エージェント出力品質バリデーション
│   ├── stop-hook.sh       ← パイプライン完了ガード
│   └── test-hooks.sh      ← 自動フックテストスイート（62テスト）
└── skills/forge/SKILL.md  ← オーケストレーター命令（メインスキル）
```
