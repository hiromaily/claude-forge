# Anthropic Go SDK 統合

ステータス: **リサーチ** (2026-04-21)

## 概要

本ドキュメントでは、[anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go)
を claude-forge に統合することで何が実現できるかを評価する。主な動機は
[ダッシュボードのリモートコントロール](./remote-dashboard-control.md) の Phase 2 であり、
Claude Code なしで forge パイプラインを実行できる Agent ランタイムが必要となる。

## 課題: Claude Code への依存

claude-forge は現在、LLM とのやり取りを全て Claude Code に依存している:

```text
Claude Code CLI
  └── Agent ツール (フェーズごとにサブエージェントを起動)
        └── 各サブエージェント: 完全な Claude Code セッション
              ├── システムプロンプト構築
              ├── ツールロード (Edit, Glob, Grep, Bash, …)
              └── MCP ツール呼び出し (forge-state)
```

これにより以下の制約がある:

1. **ヘッドレス実行不可** — インタラクティブな Claude Code セッション (ターミナルまたは IDE) なしではパイプラインを実行できない。
2. **外部タスク投入不可** — ダッシュボードはチェックポイントの監視と承認はできるが、新しいパイプラインを開始できない。
3. **CI/CD 統合不可** — GitHub Webhook や cron トリガーによる自動パイプラインが不可能。

## anthropic-sdk-go で実現できること

### 1. インプロセス Agent ランタイム (Phase 2 Task Runner)

Go SDK により、`taskrunner` パッケージ
([remote-dashboard-control.md §3.3](./remote-dashboard-control.md#_3-3-go-package-layout) で定義)
が `forge-state-mcp` プロセス内で直接 Agent セッションを実行できる:

```text
forge-state-mcp (単一 Go バイナリ)
  ├── MCP サーバー (stdio)       — 状態管理 (47 ツール)
  ├── ダッシュボードサーバー (HTTP) — SSE + チェックポイント承認
  └── Task Runner                — 新規: Agent SDK セッション
        ├── POST /api/task/submit → タスクをキューに追加
        ├── goroutine プールがタスクを取得
        ├── anthropic-sdk-go マルチターンセッション
        │     ├── tool_use: ファイル読み書き
        │     ├── tool_use: forge-state MCP 呼び出し (インプロセス)
        │     └── tool_use: シェルコマンド
        ├── 同じ EventBus にイベントを publish
        └── ダッシュボード SSE でリアルタイム進捗表示
```

主な利点: **統一されたコントロールプレーン**。SDK 実行のパイプラインとインタラクティブな
Claude Code パイプラインが同じ EventBus、SSE ストリーム、チェックポイント承認フローを共有する。

### 2. Claude Code 不要のパイプライン実行

SDK により、forge パイプラインを以下から起動できるようになる:

- **ダッシュボード Web UI** — スマートフォンから GitHub Issue URL を投入
- **CI/CD** — GitHub Actions ワークフローが `POST /api/task/submit` を呼び出し
- **Cron** — 既存のタスクキューによるスケジュール実行
- **CLI** — HTTP API を呼び出す軽量な `forge-run` コマンド

いずれも Claude Code のインストールや起動を必要としない。

### 3. きめ細かいモデル制御

SDK は Claude Code の Agent ツールでは公開されていないパラメータを提供する:

| パラメータ | Claude Code Agent | anthropic-sdk-go |
|-----------|-------------------|------------------|
| `temperature` | 設定不可 | リクエストごと |
| `max_tokens` | 設定不可 | リクエストごと |
| `tool_choice` | 設定不可 | `auto` / `any` / `tool` |
| `model` | 限定的 (`sonnet`, `opus`, `haiku`) | 任意のモデル ID |
| Extended thinking `budget_tokens` | 設定不可 | リクエストごと |
| `stop_sequences` | 設定不可 | リクエストごと |

これによりフェーズごとの最適化が可能になる:

- **Design reviewer**: 低 temperature、`tool_choice: {"type": "tool", "name": "submit_verdict"}` による構造化出力
- **ブレインストーミング**: 高 temperature
- **Implementer**: 大きなバジェットの extended thinking

### 4. 正確なトークン追跡

SDK はすべてのレスポンスで `usage.input_tokens` と `usage.output_tokens` を返す。
現在 `analytics_pipeline_summary` は出力テキスト長からトークンを推定しているが、
SDK 実行のパイプラインではトークンとコストの追跡が正確になる。

### 5. レビュー判定の構造化 Tool Use

現在、レビューエージェント (design-reviewer, task-reviewer, impl-reviewer) は
自由テキストで判定を出力し、文字列マッチング (`APPROVE`, `REVISE`, `PASS`, `FAIL`)
でパースしている。SDK では `tool_choice` で判定を強制できる:

```go
// モデルに verdict ツールの呼び出しを強制 — 自由テキストのパース不要
messages.New().
    Tool(anthropic.ToolParam{
        Name: "submit_verdict",
        InputSchema: verdictSchema, // {"verdict": "APPROVE"|"REVISE", "findings": [...]}
    }).
    ToolChoice(anthropic.ToolChoiceParam{
        Type: anthropic.ToolChoiceTypeTool,
        Name: anthropic.String("submit_verdict"),
    })
```

これにより `pipeline_report_result.go` に記載されている判定パース失敗の問題が解消される。

## SDK で置き換えられないもの

SDK はすべてのシナリオで Claude Code を置き換えるものではない:

| 機能 | Claude Code | anthropic-sdk-go |
|------|-------------|------------------|
| コンフリクト検出付きファイル編集 | 組み込み (Edit ツール) | 自前実装が必要 |
| コードベース検索 (Glob, Grep) | 組み込み | 自前実装またはシェル実行 |
| Git 操作 | 組み込み | シェル実行が必要 |
| LSP 統合 | 組み込み | 利用不可 |
| ユーザーインタラクション (AskUserQuestion) | 組み込み | ダッシュボード UI のみ |

**implementer フェーズ** (phase-5) はファイル編集とコードベースナビゲーションを多用するため、
トレードオフは以下の通り:

- **インタラクティブセッション**: Claude Code サブエージェントを継続使用 (現行方式) — 最良の開発体験、フルツールスイート
- **ヘッドレス/CI セッション**: SDK + シェル実行によるファイル操作 — エルゴノミクスは劣るが自動パイプラインとして機能

## 統合アーキテクチャ

```text
                    ┌─────────────────────────────┐
                    │     forge-state-mcp          │
                    │     (単一 Go バイナリ)        │
                    ├─────────────────────────────┤
                    │  MCP サーバー (stdio)         │ ← Claude Code インタラクティブ
                    ├─────────────────────────────┤
                    │  ダッシュボードサーバー (HTTP)  │ ← ブラウザ / スマートフォン
                    │    ├── SSE /events           │
                    │    ├── POST /api/task/submit  │
                    │    └── POST /api/checkpoint   │
                    ├─────────────────────────────┤
                    │  Task Runner                 │
                    │    ├── anthropic-sdk-go       │ ← ヘッドレスパイプライン
                    │    ├── インプロセス EventBus   │
                    │    └── インプロセス StateManager│
                    └─────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
         .specs/          EventBus         state.json
       (成果物)       (ブラウザへ SSE)    (パイプライン状態)
```

## 依存関係と Go Module への影響

`anthropic-sdk-go` を `mcp-server/go.mod` に追加:

```bash
cd mcp-server && go get github.com/anthropics/anthropic-sdk-go
```

SDK の推移的依存は最小限であり、大規模なフレームワークを引き込まない。
`forge-state-mcp` バイナリサイズの増加はごくわずかと予想される。

## 既存リサーチドキュメントとの関係

| ドキュメント | 関係 |
|------------|------|
| [ダッシュボードのリモートコントロール](./remote-dashboard-control.md) | Phase 2 §3.5 で Go SDK を推奨ランタイムとして特定。本ドキュメントはその選択で何が実現できるかを詳述。 |
| [Forge Queue 設計](./queue-design.md) | キュー設計は `claude -p` (ステートレス) を使用。SDK によりマルチターンセッションが可能になり、キュータスクが単発ではなくフルパイプラインを実行できる。 |

## 推奨

Phase 2 Task Runner の実装に `anthropic-sdk-go` を採用する。SDK は全システムを
単一 Go バイナリとして維持し、インプロセス EventBus 統合を実現できる唯一の選択肢である。
Node.js/Python サブプロセス (remote-dashboard-control.md §3.5 でフォールバックとして記載)
はランタイム依存を追加し、プロセス間イベント連携が必要になる。

## 次のステップ

1. `anthropic-sdk-go` を `mcp-server/go.mod` に追加し、基本的な API 呼び出しを検証
2. SDK ベースの Agent セッションで `mcp-server/internal/taskrunner/` を実装
3. `TaskRunner` を `dashboard.StartOptions` に接続 (remote-dashboard-control.md §3.4 に従う)
4. `POST /api/task/submit` と `GET /api/tasks` エンドポイントを追加
5. ダッシュボード UI: タスク投入フォームとタスクリストパネル
