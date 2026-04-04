# データフロー

> **注:** 工数 `L`（`full` テンプレート）の全線形フローを示します。工数レベルが低い場合（S、M）はラベル付きのフェーズがスキップされます — スキップテーブルについては[工数ドリブンフロー](effort-flow)セクションを参照してください。

```
$ARGUMENTS
    │
    ▼
┌──────────────────┐
│ 入力バリデーション │ mcp__forge-state__validate_input (決定的)
│                   │ + LLM 整合性チェック (意味的)
└──────┬───────────┘
       │ 無効 → エラーで停止
       ▼
┌──────────────────┐
│ ワークスペース     │ → request.md, state.json
│ セットアップ       │   (工数/flowTemplate を設定し、スキップされる
│ (工数を検出し、    │    フェーズごとに事前に skip-phase を呼び出す)
│  フローテンプレート│
│  を設定)          │
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ Phase 1           │ situation-analyst → analysis.md
│ Phase 2           │ investigator → investigation.md
└──────┬───────────┘
       │
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 3 ←→ Phase 3b (APPROVE/REVISE ループ)       │
│ architect → design.md                              │
│ design-reviewer → review-design.md                 │
└──────┬───────────────────────────────────────────┘
       │ チェックポイント A（人間の承認）
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 4 ←→ Phase 4b (APPROVE/REVISE ループ)       │
│ task-decomposer → tasks.md                         │
│ task-reviewer → review-tasks.md                    │
└──────┬───────────────────────────────────────────┘
       │ [phase-4b、checkpoint-b は工数 S と M でスキップ]
       │ チェックポイント B（人間の承認；工数 L のみ）
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 5-6（タスクごと、安全な場合は並列）            │
│ implementer → コードファイル + impl-{N}.md          │
│ impl-reviewer → review-{N}.md                      │
│ (FAIL → リトライ、最大 2 回)                        │
└──────┬───────────────────────────────────────────┘
       ▼
┌──────────────────────────────────────────────────┐
│ Phase 7 — 包括的レビュー                            │
│ comprehensive-reviewer → comprehensive-review.md   │
└──────┬───────────────────────────────────────────┘
       │ [phase-7 は工数 S でスキップ]
       ▼
┌──────────────────┐
│ 最終検証          │ verifier（型チェック＋テストスイート）
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ PR 作成           │ git push + gh pr create → PR #
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ 最終サマリー       │ → summary.md（PR #、改善レポートを含む）
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ ソースへの投稿     │ → GitHub/Jira コメント（該当する場合）
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│ 最終コミット       │ pipeline_report_result → state.json = "completed"
│                   │ git add summary.md state.json
│                   │ git commit --amend --no-edit
│                   │ git push --force-with-lease
│                   │ (PR ブランチに最終状態の summary.md + state.json が含まれる)
└──────────────────┘
```

## 各エージェントが読み取るもの

情報フローは厳密に前方向です — 後のフェーズのエージェントの出力を読み取るエージェントはいません。

| エージェント | ワークスペースから読み取るもの |
|-------|---------------------|
| situation-analyst | request.md |
| investigator | request.md, analysis.md |
| architect | request.md, analysis.md, investigation.md（リビジョン時は +review-design.md） |
| design-reviewer | request.md, analysis.md, investigation.md, design.md |
| チェックポイント A（オーケストレーター） | design.md, review-design.md（人間にサマリーを提示するため） |
| task-decomposer | request.md, design.md, investigation.md（リビジョン時は +review-tasks.md） |
| task-reviewer | request.md, design.md, investigation.md, tasks.md |
| チェックポイント B（オーケストレーター） | tasks.md, review-tasks.md（人間にサマリーを提示するため） |
| implementer | request.md, design.md, tasks.md, review-{dep}.md（リトライ時は +review-{N}.md） — さらにオーケストレーターが `mcp__forge-state__search_patterns`（BM25）経由で挿入する `## Similar Past Implementations` ブロック |
| impl-reviewer | request.md, tasks.md, design.md, impl-{N}.md, git diff（ファイルスコープ、main...HEAD） |
| comprehensive-reviewer | request.md, design.md, tasks.md, すべての impl-{N}.md、すべての review-{N}.md、git diff ＋ 選択的な構造読み取り |
| verifier | （フィーチャーブランチのコードを直接読み取る） |
| PR 作成（オーケストレーター） | request.md, design.md, tasks.md（PR タイトルとボディのため） |
| 最終サマリー（オーケストレーター） | 改善レポートのエピローグのために analysis.md と investigation.md を読み取る（存在する場合）；工数レベルに関係なく固定の入力ファイルリスト |
| ソースへの投稿（オーケストレーター） | summary.md, request.md（コメントターゲットのソースメタデータ） |

## ファイル書き込みの責任

- **Phase 1–4b、6**: エージェントが出力文字列を返す → オーケストレーターがファイルを書き込む
- **Phase 5**: エージェントがコードファイルと impl-{N}.md を直接書き込む（ファイルシステムの操作が必要）
- **Phase 7**: エージェントがコード修正を直接書き込み、comprehensive-review.md のコンテンツを返す
- **最終検証**: エージェントが問題を直接修正し、アーティファクトファイルなし
- **PR 作成**: オーケストレーターが直接処理（git push + gh pr create）
- **最終サマリー**: オーケストレーターが summary.md を書き込む（PR 作成で取得した PR # を含む）
- **ソースへの投稿**: オーケストレーターが直接処理（GitHub/Jira にコメントを投稿）
- **最終コミット**: オーケストレーターはまず `pipeline_report_result` を呼び出し（state.json を "completed" に進める）、次に最後のコミットを amend して summary.md + state.json を含め、その後 force-push する（PR ブランチには最終状態の summary.md + state.json が含まれる）

## スペックインデックスシステム

スペックインデックスはパイプライン間の学習を提供します — 過去の実行のパターンを表面化して現在のエージェントを導きます。

**コンポーネント：**

| コンポーネント | 役割 |
|--------|------|
| `indexer.BuildSpecsIndex` | `mcp-server/indexer/specs_index.go` の Go 関数。`.specs/` 内のすべてのワークスペースサブディレクトリをスキャンし、`.specs/index.json` を書き込む。`requestSummary`、`reviewFeedback`（`review-*.md` の REVISE 判定から）、`implOutcomes`、`implPatterns`（`impl-*.md` のファイル変更セクションから）、および `outcome` を抽出する。各パイプライン完了後に `mcp__forge-state__refresh_index` によって呼び出される。 |
| `mcp__forge-state__search_patterns` | **プライマリスコアリングパス。** MCP ツールとして公開された BM25 スコアラー。`.specs/index.json` と `{workspace}/request.md` を読み取り、BM25（長さ正規化付き IDF 重み付き用語頻度；`k1=1.5`、`b=0.75`）を使用して過去のエントリをスコアリングし、フォーマット済みマークダウンを出力する。2つのモードをサポート：**review-feedback**（デフォルト）は `## Past Review Feedback` ブロックを出力；**impl** モードは `## Similar Past Implementations` ブロックを出力する。MCP のみ — シェルフォールバックは存在しない。 |

**データフロー：**

```
完了したパイプライン
  └─► mcp__forge-state__refresh_index
        └─► indexer.BuildSpecsIndex → .specs/index.json

次のパイプライン、Phase 3:
  orchestrator → mcp__forge-state__search_patterns(workspace, top_k=3, mode="review-feedback")
    → "## Past Review Feedback" を architect プロンプトに挿入

次のパイプライン、Phase 4:
  orchestrator → mcp__forge-state__search_patterns(workspace, top_k=3, mode="review-feedback")
    → "## Past Review Feedback" を task-decomposer プロンプトに挿入

次のパイプライン、Phase 5（各タスクの前）:
  orchestrator → mcp__forge-state__search_patterns(workspace, top_k=2, mode="impl")
    → "## Similar Past Implementations" を implementer プロンプトに挿入
```

このシステムは追記のみで、エージェントの視点からは読み取り専用です。エージェントはインデックスに書き込みません；オーケストレーター経由でのみ消費します。
