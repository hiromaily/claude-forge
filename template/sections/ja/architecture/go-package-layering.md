`mcp-server/internal/` パッケージは厳格な一方向インポート DAG を形成しています。この方向に違反するとインポートサイクルとビルドの失敗が発生し、`import_cycle_test.go` がこれを強制します。

```
tools  →  orchestrator  →  state
  │              │
  └──────────────┴──→  (共有パッケージ: history, profile, prompt, validation, events)
```

## ルール

- `state` は `orchestrator` または `tools` をインポートしてはなりません。
- `orchestrator` は `tools` をインポートしてはなりません。
- `tools` はその下にある任意のパッケージをインポートできます。
- 共有パッケージ（`history`、`profile`、`prompt`、`validation`、`events`）は `state` をインポートできますが、`orchestrator` または `tools` はインポートしてはなりません。

## 理由

`state` はドメインロジックを持たない永続化レイヤーです。`orchestrator` はパイプラインステートマシン（`Engine.NextAction`）を含みます。`tools` は `orchestrator` を MCP ハンドラーでラップし、エンリッチメント（エージェントプロンプト、履歴検索）を追加します。この方向を一方向に保つことで、各レイヤーが依存対象をモックすることなくテスト可能になります。

Go MCP ハンドラーは自身の操作に対してフェイルオープンでは**ありません** — ガードの失敗は `IsError=true` を返します。ただし、MCP サーバーが利用できなくてもシェルレベルの操作はブロックされません（2つのレイヤーは独立しています）。

## 強制

`mcp-server/` の `import_cycle_test.go` はすべての `go test` 実行時に DAG を検証します。逆方向のインポートを追加すると、テストがサイクルエラーで失敗します。
