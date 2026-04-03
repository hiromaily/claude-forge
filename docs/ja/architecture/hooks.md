# Hooks & ガードレール

フックはシェルレベルで重要な制約を強制します — LLMが誤解できない決定論的ガードです。

## フックタイプ

| フック | スクリプト | トリガー |
| --- | --- | --- |
| PreToolUse | `pre-tool-hook.sh` | ツール実行前 |
| PostToolUse (Agent) | `post-agent-hook.sh` | エージェント復帰後 |
| PostToolUse (Bash) | `post-bash-hook.sh` | bashコマンド完了後 |
| Stop | `stop-hook.sh` | パイプラインが停止しようとしたとき |

## 終了コードのセマンティクス

- `exit 0` — アクションを許可
- `exit 2` — アクションをブロック（ハードストップ）

## PreToolUse ルール

### ルール1：読み取り専用ガード

Phase 1（状況分析）とPhase 2（調査）の間、ソースファイルの編集がブロックされます。ワークスペースディレクトリへのアーティファクト書き込みのみ許可されます。

```
Phase 1-2 アクティブ + Edit/Write ツール → exit 2（ブロック）
Phase 1-2 アクティブ + .specs/ への Edit/Write → exit 0（許可）
```

### ルール2：並列コミットブロック

並列Phase 5実行中、git commitがブロックされます。オーケストレーターが並列グループ終了後にバッチコミットします。

```
並列タスクアクティブ + git commit → exit 2（ブロック）
シーケンシャルタスク + git commit → exit 0（許可）
```

### ルール3：Main/Master チェックアウトブロック

アクティブなパイプライン中、`main` や `master` へのチェックアウトがブロックされ、フィーチャーブランチから離脱することを防ぎます。

```
アクティブなパイプライン + git checkout main → exit 2（ブロック）
パイプラインなし + git checkout main → exit 0（許可）
```

## PostToolUse：エージェント出力バリデーション

各エージェントの復帰後、`post-agent-hook.sh` が出力品質をチェックします：
- 出力が空または支離滅裂な場合に警告
- `status == "in_progress"` フィルターを使用（他のフックとは異なる）

## 最終コミットステップ

PR作成と summary.md 生成の後、パイプラインは最終コミットを amend して `summary.md` と `state.json` を含め、force-push します。これにより PR ブランチに PR 番号を含む最終サマリーが反映されます。

v1（シェルベース）では `post-bash-hook.sh` が `post-to-source` フェーズ後に自動コミットしていました。v2（MCP駆動）では Engine が明示的な `exec` アクションとして amend + force-push を発行します。

## Stopフック：完了ガード

`stop-hook.sh` はパイプラインの途中終了を防ぎます — パイプラインは `post-to-source` フェーズを完了して `summary.md` を作成するか、明示的に中止される必要があります。

## 共有ヘルパー

`scripts/common.sh` は `find_active_workspace` を提供 — `pre-tool-hook.sh` と `stop-hook.sh` が使用。注意：`post-agent-hook.sh` は異なるフィルターを使用し、`common.sh` をソースしません。

## テスト

```bash
# フルフックテストスイートの実行（58テスト）
bash scripts/test-hooks.sh

# サンプル入力での手動テスト
echo '{"tool_name":"Edit","tool_input":{"file_path":"/src/foo.go"}}' \
  | bash scripts/pre-tool-hook.sh
echo $?  # 0（パイプラインなし）または 2（ブロック）
```
