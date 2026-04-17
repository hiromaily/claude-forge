## 前提条件

- **Claude Code** — CLIがインストール済みであること
- **jq** — フックスクリプトに必要（macOSでは `brew install jq`）
- **Go** — ローカル開発ビルドにのみ必要（プラグインインストールでは不要）

## プラグインインストール（推奨）

プラグインをインストールすると、すべてが自動的に設定されます：

```bash
# マーケットプレイスを登録（初回のみ）
/plugin marketplace add hiromaily/claude-forge

# プラグインをインストール
/plugin install claude-forge
/reload-plugins
```

インストール後、Claude Codeを再起動して確認：

```bash
/mcp   # forge-state が Connected と表示されるはず
```

### 自動的に行われること

プラグインがインストールされると、Claude Codeは：

1. **MCPサーバーを登録** — `.mcp.json` が `forge-state` サーバーを宣言
2. **Setupフックを実行** — GitHub Releasesからビルド済み `forge-state-mcp` バイナリをダウンロード
3. **ソースビルドにフォールバック** — リリースバイナリが利用不可の場合、`go build` でソースからビルド

```
plugin.json                        ← "mcpServers": "./.mcp.json" を宣言
  └─> .mcp.json                    ← forge-state サーバー定義（stdioトランスポート）
        └─> scripts/launch-mcp.sh  ← 自己修復ランチャー
              └─> bin/forge-state-mcp ← Setupフックでダウンロードされたバイナリ
```

### 代替インストール方法

```bash
# ローカルクローンからインストール
claude plugins install ~/path/to/claude-forge

# ワンタイムセッション（永続インストールなし）
claude --plugin-dir ~/path/to/claude-forge
```

## ローカル開発セットアップ

claude-forge自体の開発に携わるコントリビューター向け：

```bash
# バイナリをビルド・インストールし、Claude Codeに登録
make setup-manual
```

`--scope local` でMCPサーバーが登録されます（`.claude/settings.local.json` に書き込み、gitignore済み）。

## 環境変数

| 変数 | 必須 | 説明 |
| --- | --- | --- |
| `FORGE_AGENTS_PATH` | はい | `agents/` ディレクトリの絶対パス。`make setup` で自動設定。 |
| `FORGE_SPECS_DIR` | いいえ | デフォルトの `.specs/` ディレクトリを上書き。 |
| `FORGE_EVENTS_PORT` | いいえ | SSE イベントエンドポイントおよび同梱の Web ダッシュボード（`http://localhost:<port>/`）用のポート。 |

## トラブルシューティング

### forge-state が2つ表示され、1つが失敗

claude-forge開発リポジトリ内でプラグインもインストールされている場合、`make setup-manual` を実行してローカルスコープのオーバーライドを登録してください。

### forge-state が "Failed to connect" と表示

1. バイナリの存在を確認
2. `FORGE_AGENTS_PATH` が有効なディレクトリを指していることを確認
3. バイナリを直接テスト：`echo '{}' | forge-state-mcp`
4. セットアップを再実行

### Setupフックが実行されなかった

バージョンマーカーを削除して再ダウンロードをトリガー：

```bash
rm -f $(claude plugins path)/claude-forge/bin/.installed-version
```

## アップデート

```bash
claude plugin update claude-forge@claude-forge
/reload-plugins
# MCPサーバーをリロードするにはClaude Codeを再起動
```

## アンインストール

```bash
claude plugins uninstall claude-forge@claude-forge
# 手動登録した場合：
claude mcp remove forge-state -s user
```
