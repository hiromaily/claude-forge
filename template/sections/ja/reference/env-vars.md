## 必須

### `FORGE_AGENTS_PATH`

`agents/` ディレクトリの絶対パス。`pipeline_next_action` がランタイムでエージェントの `.md` ファイルを解決するために必要。

`make setup` で自動設定。手動セットアップの場合は `claude mcp add --env` で渡します。

## オプション

### `FORGE_SPECS_DIR`

エンジンが使用するデフォルトの `.specs/` ディレクトリを上書き。テストや異なる場所で複数のパイプラインを実行する際に有用。

デフォルト：`.specs/`（プロジェクトルートからの相対パス）

### `FORGE_EVENTS_PORT`

SSEイベントエンドポイントのポート。設定すると、`subscribe_events` MCPツールがリアルタイムパイプラインイベント監視用のSSE URLを返します。

デフォルト：未設定（SSE無効）

## セットアップ

環境変数は `make setup` 使用時に自動設定されます。手動セットアップの場合：

```bash
claude mcp add forge-state \
  --scope user \
  --transport stdio \
  --cmd forge-state-mcp \
  --env FORGE_AGENTS_PATH=/path/to/agents
```
