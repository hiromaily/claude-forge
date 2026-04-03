# 状態管理

## 概要

すべてのパイプライン状態は **Go MCPサーバー**（`forge-state`）を通じて管理され、44の型付きツールコールを公開しています。状態はワークスペースディレクトリ内の `state.json` に永続化されます。

## ステートマシン

```mermaid
stateDiagram-v2
    [*] --> initialized: init
    initialized --> phase_1: phase-start
    phase_1 --> phase_2: phase-complete → phase-start
    phase_2 --> phase_3: phase-complete → phase-start
    phase_3 --> phase_3b: phase-complete → phase-start
    phase_3b --> checkpoint_a: APPROVE → checkpoint
    phase_3b --> phase_3: REVISE → revision-bump
    checkpoint_a --> phase_4: 承認 → phase-complete
    checkpoint_a --> phase_3: 却下
    phase_4 --> phase_4b: phase-complete → phase-start
    phase_4b --> checkpoint_b: APPROVE → checkpoint
    phase_4b --> phase_4: REVISE → revision-bump
    checkpoint_b --> phase_5: 承認 → task-init
    phase_5 --> phase_6: タスクごと
    phase_6 --> phase_5: FAIL（リトライ）
    phase_6 --> phase_7: 全てPASS
    phase_7 --> final_verification: phase-complete
    final_verification --> pr_creation: phase-complete
    pr_creation --> final_summary: phase-complete
    final_summary --> post_to_source: phase-complete
    post_to_source --> [*]

    initialized --> abandoned: abandon
    phase_1 --> abandoned: abandon
    phase_5 --> abandoned: abandon
```

## state.json の構造

`state.json` の主要フィールド：

| フィールド | 説明 |
| --- | --- |
| `specName` | ワークスペース名（例：`20260320-fix-auth`） |
| `workspace` | `.specs/{specName}/` のフルパス |
| `status` | 現在のステータス：`initialized`、`in_progress`、`completed`、`failed`、`abandoned` |
| `currentPhase` | アクティブなフェーズID（例：`phase-3`、`checkpoint-a`） |
| `effort` | 工数レベル：`S`、`M`、`L`（XSはサポート外） |
| `flowTemplate` | 選択されたテンプレート：`light`、`standard`、`full` |
| `branch` | Gitブランチ名 |
| `autoApprove` | ブーリアン、`--auto` フラグで設定（デフォルト：`false`） |
| `phases` | フェーズレコードの配列（ステータス、タイムスタンプ、ログ） |
| `tasks` | タスクレコードの配列（実装/レビューステータス） |
| `revisions` | アーティファクトごとのリビジョンカウンター |
| `skippedPhases` | フローテンプレートによりスキップされたフェーズ（例：`["phase-4b", "checkpoint-b", "phase-7"]`） |
| `phaseLog` | フェーズメトリクスの配列：`{phase, tokens, duration_ms, model, timestamp}` |

## MCPツールカテゴリ

Go MCPサーバーは8カテゴリにわたって **44の型付きツールコール** を公開しています：

| カテゴリ | ツール数 | 説明 |
| --- | --- | --- |
| ライフサイクル | 5 | パイプラインの初期化と進行（`init`、`pipeline_init`、`pipeline_next_action` など） |
| フェーズ管理 | 6 | フェーズ遷移（`phase_start`、`phase_complete`、`checkpoint`、`abandon` など） |
| リビジョン制御 | 4 | APPROVE/REVISEサイクル管理（`revision_bump`、`inline_revision_bump` など） |
| 設定 | 7 | ランタイム設定（`set_effort`、`set_auto_approve`、`set_branch` など） |
| タスク管理 | 2 | タスクごとの追跡（`task_init`、`task_update`） |
| メトリクス & クエリ | 9 | 状態クエリ、履歴検索、BM25パターンマッチング |
| 分析 | 3 | パイプライン統計とコスト予測 |
| バリデーション & ユーティリティ | 8 | 入力/アーティファクトバリデーション、AST分析、依存グラフ |

完全なツールリファレンスは [MCPツール](/ja/reference/mcp-tools) を参照してください。

### MCPハンドラーガード

MCPサーバーは以下のガードを決定論的に強制します（フック経由ではなく）：

| ガード | ツール | 条件 |
|--------|------|------|
| アーティファクト必須 | `phase_complete` | 期待されるアーティファクトファイルが欠落している場合ブロック |
| チェックポイント必須 | `phase_complete` | `awaiting_human` ステータスが設定されていない場合ブロック |
| フェーズ順序 | `phase_start` | 前のフェーズが完了していない場合ブロック |
