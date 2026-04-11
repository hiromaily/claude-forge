## 概要図

```mermaid
flowchart TD
    START(["forge 開始"])
    START --> PI["pipeline_init"]
    PI --> RESUME{resume_mode<br>= auto?}
    RESUME -->|はい| RI["resume_info"]
    RESUME -->|いいえ| ERR{errors?}
    ERR -->|あり| REJECT(["拒否"])
    ERR -->|なし| FETCH{fetch_needed?}
    FETCH -->|あり| EXT["外部コンテキスト取得<br>GitHub / Jira"]
    FETCH -->|なし| PIC1["pipeline_init_with_context"]
    EXT --> PIC1
    PIC1 --> CONFIRM["ユーザーが effort + slug を確認"]
    CONFIRM --> PIC2["pipeline_init_with_context<br>with user_confirmation"]
    PIC2 --> SETUP["setup フェーズ:<br>init, request.md 作成,<br>effort/template 設定, task_init"]
    RI --> LOOP

    SETUP --> LOOP

    LOOP["pipeline_next_action"]
    LOOP --> TYPE{action.type}

    TYPE -->|spawn_agent| AGENT["エージェント実行"]
    TYPE -->|checkpoint| CP["checkpoint + ユーザーに提示"]
    TYPE -->|exec| EXEC["コマンド実行"]
    TYPE -->|write_file| WF["ファイル書き込み"]
    TYPE -->|done: skip| SKIP["スキップされたフェーズの phase_complete"]
    TYPE -->|done| DONE(["完了"])

    AGENT --> RPT["pipeline_report_result"]
    EXEC --> RPT
    WF --> RPT
    CP --> PC["phase_complete"]
    SKIP --> LOOP

    RPT --> HINT{next_action_hint}
    HINT -->|revision_required| USER["ユーザーに結果を提示"]
    HINT -->|setup_continue| LOOP
    HINT -->|normal| LOOP
    PC --> LOOP
    USER --> LOOP
```

## フェーズテーブル

実行順の18フェーズ。effort レベル（フローテンプレート）に応じてスキップされるフェーズあり。

| # | フェーズ ID | 説明 | アクター | 成果物 |
|---|----------|-------------|-------|----------|
| 1 | `setup` | ワークスペース初期化、request.md 作成、effort 検出、テンプレート設定 | オーケストレーター | request.md, state.json |
| 2 | `phase-1` | 状況分析 — 読み取り専用のコードベースマッピング | situation-analyst | analysis.md |
| 3 | `phase-2` | 調査 — 詳細調査、エッジケース | investigator | investigation.md |
| 4 | `phase-3` | 設計 — アーキテクチャとアプローチ | architect | design.md |
| 5 | `phase-3b` | 設計レビュー — AI 品質ゲート | design-reviewer | review-design.md |
| 6 | `checkpoint-a` | 設計の人間によるレビュー | ユーザー | 承認 / 修正 |
| 7 | `phase-4` | タスク分解 — 番号付きタスクリスト | task-decomposer | tasks.md |
| 8 | `phase-4b` | タスクレビュー — AI 品質ゲート | task-reviewer | review-tasks.md |
| 9 | `checkpoint-b` | タスクの人間によるレビュー | ユーザー | 承認 / 修正 |
| 10 | `phase-5` | 実装 — タスクごとの TDD（逐次または並列） | implementer | impl-N.md |
| 11 | `phase-6` | コードレビュー — タスクごと、最大2回リトライ | impl-reviewer | review-N.md |
| 12 | `phase-7` | 包括的レビュー — 横断的な懸念事項 | comprehensive-reviewer | comprehensive-review.md |
| 13 | `final-verification` | フルビルド + テストスイート検証 | verifier | final-verification.md |
| 14 | `pr-creation` | `gh pr create` による PR 作成（summary.md は未生成） | オーケストレーター | PR URL |
| 15 | `final-summary` | PR 番号・実行統計・改善レポートを含む summary.md 生成 | オーケストレーター | summary.md |
| 16 | `final-commit` | PR body を summary.md で更新 + 最終コミット amend + force-push | オーケストレーター | — |
| 17 | `post-to-source` | GitHub/Jira Issue にサマリーを投稿 | オーケストレーター | Issue コメント |
| 18 | `completed` | パイプライン完了 | — | — |

## Effort レベルとスキップされるフェーズ

| Effort | フローテンプレート | スキップされるフェーズ |
|--------|---------------|----------------|
| S | light | phase-4b（タスクレビュー）、checkpoint-b（タスクチェックポイント）、phase-7（包括的レビュー） |
| M | standard | phase-4b（タスクレビュー）、checkpoint-b（タスクチェックポイント） |
| L | full | _（なし）_ |

## シーケンス図 — オーケストレーター / MCP サーバー間のやり取り

```mermaid
sequenceDiagram
    actor User as ユーザー
    participant Orch as オーケストレーター<br>(SKILL.md)
    participant MCP as MCP サーバー<br>(forge-state)
    participant Agent as サブエージェント
    participant FS as .specs/

    Note over User,FS: Step 1 — 初期化または再開

    User->>Orch: /forge <引数>
    Orch->>MCP: pipeline_init(arguments)
    MCP-->>Orch: PipelineInitResult

    alt resume_mode = "auto"
        Orch->>MCP: resume_info(workspace)
        MCP-->>Orch: ResumeInfoResult
        Note over Orch: Step 2 へスキップ
    else 新規パイプライン
        opt fetch_needed（GitHub/Jira）
            Orch->>Orch: 外部コンテキスト取得
        end
        Orch->>MCP: pipeline_init_with_context(workspace, flags)
        MCP-->>Orch: needs_user_confirmation
        Orch->>User: effort オプションを提示
        User-->>Orch: effort + slug を確認
        Orch->>MCP: pipeline_init_with_context(+ user_confirmation)
        MCP->>FS: state.json, request.md 作成
        MCP-->>Orch: 確定された workspace
    end

    Note over User,FS: Step 2 — メインループ

    loop 完了まで繰り返し
        Orch->>MCP: pipeline_next_action(workspace)
        MCP-->>Orch: Action{type, phase, prompt, ...}

        alt type = spawn_agent
            Orch->>Agent: Agent(prompt)
            Agent->>FS: 成果物書き込み
            Agent-->>Orch: 結果
            Orch->>MCP: pipeline_report_result(phase, tokens, duration)
            MCP->>FS: state.json 更新、成果物検証
            MCP-->>Orch: next_action_hint

        else type = checkpoint
            Orch->>MCP: checkpoint(workspace, phase)
            MCP->>FS: status = awaiting_human 設定
            Orch->>User: レビュー依頼
            User-->>Orch: 承認 / フィードバック
            Orch->>MCP: phase_complete(phase)

        else type = exec
            Orch->>Orch: コマンド実行（git, task_init 等）
            Orch->>MCP: pipeline_report_result(phase, tokens, duration)
            MCP-->>Orch: next_action_hint

        else type = done（skip）
            Orch->>MCP: phase_complete(スキップされたフェーズ)

        else type = done
            Note over Orch: パイプライン完了
        end
    end
```

## リビジョンループの詳細

設計（phase-3/3b）とタスク（phase-4/4b）フェーズでは、AI レビュワーが REVISE 判定を返した場合にリビジョンループが発生します。ループあたり最大2回のリビジョン。

```mermaid
sequenceDiagram
    participant Orch as オーケストレーター
    participant MCP as MCP サーバー
    participant A as Architect / Decomposer
    participant R as Design / Tasks Reviewer

    Orch->>MCP: pipeline_next_action
    MCP-->>Orch: spawn_agent (architect)
    Orch->>A: 設計タスク
    A-->>Orch: design.md
    Orch->>MCP: pipeline_report_result

    Orch->>MCP: pipeline_next_action
    MCP-->>Orch: spawn_agent (design-reviewer)
    Orch->>R: design.md レビュー
    R-->>Orch: review-design.md (REVISE)
    Orch->>MCP: pipeline_report_result
    MCP-->>Orch: next_action_hint = revision_required

    Note over Orch: 結果を提示、リビジョンカウンター増加
    Orch->>MCP: pipeline_next_action
    MCP-->>Orch: spawn_agent (architect, revision 2)
    Note over Orch: ループ繰り返し（最大2回）
```

## 実装ループの詳細

各タスクは実装（phase-5）とコードレビュー（phase-6）を経ます。
レビュー失敗時は最大2回リトライ。

```mermaid
sequenceDiagram
    participant Orch as オーケストレーター
    participant MCP as MCP サーバー
    participant Impl as Implementer
    participant Rev as Impl Reviewer

    loop 各タスク
        Orch->>MCP: pipeline_next_action
        MCP-->>Orch: spawn_agent (implementer, task N)
        Orch->>Impl: タスク N 実装
        Impl-->>Orch: impl-N.md
        Orch->>MCP: pipeline_report_result

        Orch->>MCP: pipeline_next_action
        MCP-->>Orch: spawn_agent (impl-reviewer, task N)
        Orch->>Rev: impl-N.md レビュー
        Rev-->>Orch: review-N.md

        alt PASS / PASS_WITH_NOTES
            Orch->>MCP: pipeline_report_result
            Note over Orch: 次のタスクへ
        else FAIL（リトライ < 2）
            Orch->>MCP: pipeline_report_result
            Note over Orch: タスク N をリトライ
        end
    end
```
