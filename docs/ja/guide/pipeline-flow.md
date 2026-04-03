# パイプラインフロー

## 概要図

```mermaid
flowchart TD
    START(["▶ /forge"])
    START --> RC{state.json<br>が存在?}
    RC -->|はい| RESUME[state.jsonを読み込み<br>変数を復元]
    RC -->|いいえ| IV["🛡️ 入力バリデーション"]
    IV -->|無効| REJECT(["❌ 拒否"])
    IV -->|有効| WS[ワークスペースセットアップ]
    RESUME --> REJOIN(("再開"))
    WS --> TE["🔍 タスクタイプ & 工数を検出"]
    TE --> P1

    REJOIN -.-> P1
    P1["Phase 1 — 状況分析"]
    P1 -->|analysis.md| P2
    P2["Phase 2 — 調査"]

    P2 -->|investigation.md| P3
    P3["Phase 3 — 設計"]
    P3 -->|design.md| P3R
    P3R["Phase 3b — 設計レビュー"]
    P3R -->|review-design.md| DREV{APPROVE?}
    DREV -->|REVISE| P3
    DREV -->|APPROVE| CPA

    CPA{{"👤 チェックポイント A"}}
    CPA -->|承認| P4
    CPA -->|却下| P3

    P4["Phase 4 — タスク分解"]
    P4 -->|tasks.md| P4R
    P4R["Phase 4b — タスクレビュー"]
    P4R -->|review-tasks.md| TREV{APPROVE?}
    TREV -->|REVISE| P4
    TREV -->|APPROVE| CPB

    CPB{{"👤 チェックポイント B"}}
    CPB -->|承認| GITBR["フィーチャーブランチ作成"]
    CPB -->|却下| P4

    GITBR --> P5

    subgraph loop ["🔄 タスクごと"]
        P5["Phase 5 — 実装"]
        P5 -->|impl-N.md| P6
        P6["Phase 6 — コードレビュー"]
        P6 -->|review-N.md| RESULT{PASS?}
        RESULT -->|"FAIL (最大2回リトライ)"| P5
    end
    RESULT -->|全てPASS| P7

    P7["Phase 7 — 包括的レビュー"]
    P7 --> FV["最終検証"]
    FV --> PR["PR作成"]
    PR --> FS["最終サマリー<br>(PR番号を含む)"]
    FS --> FC["最終コミット<br>amend + force-push"]
    FC --> DONE(["✔ 完了"])
```

## フェーズテーブル

| フェーズ | タスク | エージェント | 入力 | 出力 | 人間の介入 |
| ----- | ---- | ----- | ----- | ------ | ----- |
| 0 | 入力バリデーション | validate-input + LLM | ユーザー入力 | バリデーション結果 | なし |
| 1 | ワークスペースセットアップ | オーケストレーター | 検証済み入力 | request.md, state.json | あり |
| 2 | タスクタイプ & 工数検出 | オーケストレーター | request.md | state.json | あり |
| 3 | 状況分析 | situation-analyst | request.md | analysis.md | なし |
| 4 | 調査 | investigator | analysis.md | investigation.md | なし |
| 5 | 設計 | architect | investigation.md | design.md | なし |
| 6 | 設計レビュー | design-reviewer | design.md | review-design.md | なし |
| 7 | チェックポイント A | 人間 | design.md | 承認 / 修正 | あり |
| 8 | タスク分解 | task-decomposer | design.md | tasks.md | なし |
| 9 | タスクレビュー | task-reviewer | tasks.md | review-tasks.md | なし |
| 10 | チェックポイント B | 人間 | tasks.md | 承認 / 修正 | あり |
| 11 | 実装 | implementer | タスク仕様 | impl-N.md | なし |
| 12 | コードレビュー | impl-reviewer | impl-N.md | review-N.md | なし |
| 13 | 包括的レビュー | comprehensive-reviewer | 全impl + レビュー | comprehensive-review.md | なし |
| 14 | 最終検証 | verifier | comprehensive-review.md | 検証結果 | なし |
| 15 | PR作成 | オーケストレーター | コミット | PR（PR番号確定） | なし |
| 16 | 最終サマリー | オーケストレーター | 全アーティファクト + PR番号 | summary.md（PR番号を含む） | なし |
| 17 | 最終コミット | オーケストレーター | summary.md, state.json | 最終commitをamend + force-push | なし |
| 18 | ソースへ投稿 | オーケストレーター | summary.md | Issueコメント | なし |
| 19 | 完了 | システム | summary.md | — | なし |

## シーケンス図

```mermaid
sequenceDiagram
    actor User as ユーザー
    participant Orch as オーケストレーター
    participant SM as MCPサーバー
    participant Hook as フック
    participant FS as ワークスペース (.specs/)
    participant Agent as サブエージェント

    User->>Orch: /forge <引数>
    Orch->>Orch: validate_input + LLMチェック

    Orch->>SM: init {workspace}
    SM->>FS: state.json作成
    Orch->>FS: request.md書き込み

    rect rgb(230, 245, 255)
    Note over Orch,Agent: Phase 1-2 — 分析（読み取り専用）
    Orch->>SM: phase-start phase-1
    Orch->>Agent: situation-analyst
    Note over Hook: Edit/Writeをブロック
    Agent-->>Orch: 分析出力
    Orch->>FS: analysis.md書き込み
    Orch->>SM: phase-complete phase-1
    end

    rect rgb(255, 245, 230)
    Note over Orch,Agent: Phase 3/3b — 設計 + レビューループ
    loop 最大2回の修正
        Orch->>Agent: architect → design.md
        Orch->>Agent: design-reviewer → review-design.md
        break APPROVE
        end
    end
    end

    rect rgb(255, 230, 230)
    Note over Orch,User: チェックポイント A — 人間のレビュー
    Orch->>User: 設計を提示
    User-->>Orch: 承認 / フィードバック
    end

    rect rgb(230, 255, 230)
    Note over Orch,Agent: Phase 5/6 — タスクごとの実装
    loop 各タスク
        Orch->>Agent: implementer → impl-N.md
        Orch->>Agent: impl-reviewer → review-N.md
    end
    end

    Orch->>Agent: comprehensive-reviewer
    Orch->>Agent: verifier
    Orch->>Orch: git push + gh pr create
    Note over Orch: PR番号が確定
    Orch->>FS: summary.md書き込み（PR番号を含む）
    Orch->>Orch: git commit --amend --no-edit
    Orch->>Orch: git push --force-with-lease
    Note over Orch: PRブランチにsummary.mdが含まれる
    Orch->>User: 完了
```

## タスクタイプ

タスクタイプによって特定のフェーズがスキップされます：

| タスクタイプ | 説明 | スキップされるフェーズ |
| --- | --- | --- |
| `feature` | 新機能や動作の追加 | _（なし — フルパイプライン）_ |
| `bugfix` | 再現手順が明確なバグ修正 | 設計レビュー (3b)、タスク分解 (4)、タスクレビュー (4b)、包括的レビュー (7) |
| `refactor` | 動作変更を伴わないコード再構成 | 設計レビュー (3b)、包括的レビューは異なる基準を使用 |
| `docs` | ドキュメントのみの変更 | 調査 (2)、設計 (3)、設計レビュー (3b)、タスク分解 (4)、タスクレビュー (4b) |
| `investigation` | 分析のみ — コード変更なし | 全実装フェーズ (5-7, 14-15) — 分析のみ出力 |
