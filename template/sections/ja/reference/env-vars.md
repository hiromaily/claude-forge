## 必須

### `FORGE_AGENTS_PATH`

`agents/` ディレクトリの絶対パス。`pipeline_next_action` がランタイムでエージェントの `.md` ファイルを解決するために必要。

`make setup` で自動設定。手動セットアップの場合は `claude mcp add --env` で渡します。

## オプション

### `FORGE_SPECS_DIR`

エンジンが使用するデフォルトの `.specs/` ディレクトリを上書き。テストや異なる場所で複数のパイプラインを実行する際に有用。

デフォルト：`.specs/`（プロジェクトルートからの相対パス）

### `FORGE_EVENTS_PORT`

SSE イベントエンドポイント**および**同梱の Web ダッシュボード用のポート。設定すると MCP サーバはローカル HTTP リスナを起動し、以下を配信します:

- `GET /events` — `subscribe_events` MCP ツールが利用する Server-Sent Events ストリーム
- `GET /` — `/events` を購読してパイプラインのフェーズ遷移をリアルタイム描画する依存ゼロのダッシュボード（単一の埋め込み HTML）

任意のパイプライン起動後にブラウザで `http://localhost:<port>/` を開いてください。ストリーム切断時は自動再接続し、ワークスペース別フィルタにも対応します。

設定ポートが使用中の場合、**8100〜8200** の範囲でランダムなポートを自動リトライします。実際の URL は stderr に出力されます。HTTP リスナは `127.0.0.1` のみにバインドします。

デフォルト：`8099`（プラグインインストール時は `.mcp.json` で設定済み。未設定 = HTTP リスナ無効）

## セットアップ

環境変数は `make setup` 使用時に自動設定されます。手動セットアップの場合：

```bash
claude mcp add forge-state \
  --scope user \
  --transport stdio \
  --cmd forge-state-mcp \
  --env FORGE_AGENTS_PATH=/path/to/agents
```
