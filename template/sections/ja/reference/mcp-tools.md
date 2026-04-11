`forge-state` MCPサーバーは **44の型付きツールコール** を公開しています。ツール名にはアンダースコアを使用（MCPプロトコル要件）。

## ライフサイクル

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__init` | 新規ワークスペースとstate.jsonを作成 |
| `mcp__forge-state__pipeline_init` | フルコンテキストでパイプラインを初期化 |
| `mcp__forge-state__pipeline_init_with_context` | 外部コンテキスト（Jira/GitHub）で初期化 |
| `mcp__forge-state__pipeline_next_action` | オーケストレーターの次のアクションを取得 |
| `mcp__forge-state__pipeline_report_result` | フェーズ結果を報告しパイプラインを進める |

## フェーズ管理

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__phase_start` | フェーズを開始 |
| `mcp__forge-state__phase_complete` | フェーズを完了（アーティファクトガード強制） |
| `mcp__forge-state__phase_fail` | フェーズの失敗を記録 |
| `mcp__forge-state__checkpoint` | ヒューマンチェックポイントに入る |
| `mcp__forge-state__skip_phase` | フェーズをスキップ |
| `mcp__forge-state__abandon` | パイプラインを中止 |

## リビジョン制御

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__revision_bump` | フルリビジョンサイクル（フェーズ再実行） |
| `mcp__forge-state__inline_revision_bump` | 再実行なしの軽微な修正 |
| `mcp__forge-state__set_revision_pending` | リビジョンをペンディングとしてマーク |
| `mcp__forge-state__clear_revision_pending` | ペンディングリビジョンをクリア |

## 設定

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__set_branch` | Gitブランチ名を設定 |
| `mcp__forge-state__set_effort` | 工数レベル（S/M/L）を設定 |
| `mcp__forge-state__set_flow_template` | フローテンプレート（light/standard/full）を設定 |
| `mcp__forge-state__set_auto_approve` | チェックポイントの自動承認を有効化 |
| `mcp__forge-state__set_skip_pr` | PR作成をスキップ |
| `mcp__forge-state__set_debug` | デバッグモードを有効化 |
| `mcp__forge-state__set_use_current_branch` | 新規作成せず現在のブランチを使用 |

## タスク管理

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__task_init` | tasks.mdからタスクリストを初期化 |
| `mcp__forge-state__task_update` | タスクの実装/レビューステータスを更新 |

## メトリクス

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__phase_log` | フェーズメトリクスを記録（トークン数、時間、モデル） |
| `mcp__forge-state__phase_stats` | フェーズ統計を取得 |

## クエリ

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__get` | 現在のパイプライン状態を取得 |
| `mcp__forge-state__resume_info` | 中断されたパイプラインの再開情報を取得 |
| `mcp__forge-state__search_patterns` | 過去のパイプライン仕様インデックスをBM25検索 |
| `mcp__forge-state__subscribe_events` | SSEエンドポイントURLを取得（`FORGE_EVENTS_PORT` が必要） |
| `mcp__forge-state__profile_get` | キャッシュされたリポジトリプロファイルを取得 |
| `mcp__forge-state__history_search` | 過去のパイプライン履歴を検索 |
| `mcp__forge-state__history_get_patterns` | 蓄積されたレビュー指摘パターンを取得 |
| `mcp__forge-state__history_get_friction_map` | 改善レポートからのAIフリクションポイントを取得 |

## 分析

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__analytics_pipeline_summary` | 単一実行のトークン、時間、コスト統計 |
| `mcp__forge-state__analytics_repo_dashboard` | 全パイプライン実行の集計統計 |
| `mcp__forge-state__analytics_estimate` | 新規実行のP50/P90予測 |

## コード分析

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__ast_summary` | ソースファイルのTree-sitter ASTサマリー |
| `mcp__forge-state__ast_find_definition` | シンボルの定義を特定して返す |
| `mcp__forge-state__dependency_graph` | ファイルレベルのインポートグラフをJSON形式で |
| `mcp__forge-state__impact_scope` | 指定シンボルを呼び出すファイルを検索 |

## バリデーション & ユーティリティ

| MCPツール | 説明 |
|---|---|
| `mcp__forge-state__validate_input` | パイプライン入力をバリデーション（空、短すぎ、URLフォーマット） |
| `mcp__forge-state__validate_artifact` | アーティファクトの存在とコンテンツ制約を確認 |
| `mcp__forge-state__refresh_index` | `.specs/index.json` をリフレッシュ |

## ガード（MCPハンドラーで強制）

MCPサーバーはこれらのガードを決定論的に強制します：

| ガード | ツール | 条件 |
|--------|------|------|
| アーティファクト必須 | `phase_complete` | 期待されるアーティファクトファイルが欠落している場合ブロック |
| チェックポイント必須 | `phase_complete` | `awaiting_human` ステータスが未設定の場合ブロック |
| フェーズ順序 | `phase_start` | 前のフェーズが未完了の場合ブロック |
