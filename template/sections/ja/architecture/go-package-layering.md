`mcp-server/internal/` パッケージは厳格な一方向インポート DAG を形成しています。この方向に違反するとインポートサイクルとビルドの失敗が発生し、`import_cycle_test.go` がこれを強制します。

```
tools  →  orchestrator  →  state
  │            │               ↑
  │            ↓               │
  ├──→  sourcetype  ──→  maputil
  │
  └──→  (共有: history, profile, prompt, validation, events)
```

## パッケージ一覧

| パッケージ | 役割 | インポート可能 |
|-----------|------|--------------|
| `state` | 永続化レイヤー — `State` 構造体、`StateManager`、フェーズ定数、アーティファクト名 | 標準ライブラリのみ |
| `maputil` | 汎用 map フィールド抽出（`StringField`、`IntFieldAlt`、`StringArray`、`ToMap`） | 標準ライブラリのみ（リーフパッケージ） |
| `sourcetype` | ソースタイプ Handler インターフェース + レジストリ（GitHub, Jira, Linear） — URL 分類、fetch/post 設定、外部コンテキスト解析 | `state`、`maputil` |
| `orchestrator` | パイプラインステートマシン（`Engine.NextAction`）、アクション型、エフォート検出 | `state`、`sourcetype` |
| `tools` | MCP ハンドラー（`orchestrator` をラップし、エージェントプロンプト・履歴検索を付加） | `state`、`sourcetype`、`maputil`、`orchestrator`、共有パッケージ |
| `validation` | 入力バリデーション（URL 形式、フラグ、長さチェック） | `sourcetype` |
| 共有（`history`、`profile`、`prompt`、`events`） | 横断的ユーティリティ | `state` |

## ルール

- `state` は `orchestrator`、`tools`、`sourcetype` をインポートしてはなりません。
- `maputil` は内部パッケージをインポートしてはなりません（リーフパッケージ）。
- `sourcetype` は `orchestrator` または `tools` をインポートしてはなりません。
- `orchestrator` は `tools` をインポートしてはなりません。
- `tools` はその下にある任意のパッケージをインポートできます。
- 共有パッケージ（`history`、`profile`、`prompt`、`validation`、`events`）は `state` と `sourcetype` をインポートできますが、`orchestrator` または `tools` はインポートしてはなりません。

## 新しいソースタイプの追加

新しいソースタイプ（例: Asana）の追加に必要なのは **1ファイル + 1登録** のみ:

1. `internal/sourcetype/asana.go` を作成し、`Handler` インターフェースを実装
2. そのファイルに `func init() { register(&AsanaHandler{}) }` を追加

`Handler` インターフェースがコンパイル時にすべての必須メソッドを強制します。他のファイルの変更は不要です — `validation`、`tools`、`orchestrator` はすべて `sourcetype` レジストリ経由でディスパッチします。

## 理由

`state` はドメインロジックを持たない永続化レイヤーです。`maputil` は純粋なユーティリティリーフパッケージです。`sourcetype` はすべてのソースタイプ固有の知識を単一のインターフェースの背後に集約し、散在する switch 文を排除します。`orchestrator` はパイプラインステートマシン（`Engine.NextAction`）を含みます。`tools` は `orchestrator` を MCP ハンドラーでラップし、エンリッチメント（エージェントプロンプト、履歴検索）を追加します。この方向を一方向に保つことで、各レイヤーが依存対象をモックすることなくテスト可能になります。

Go MCP ハンドラーは自身の操作に対してフェイルオープンでは**ありません** — ガードの失敗は `IsError=true` を返します。ただし、MCP サーバーが利用できなくてもシェルレベルの操作はブロックされません（2つのレイヤーは独立しています）。

## 強制

`mcp-server/` の `import_cycle_test.go` はすべての `go test` 実行時に DAG を検証します。逆方向のインポートを追加すると、テストがサイクルエラーで失敗します。
