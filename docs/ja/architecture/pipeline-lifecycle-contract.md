# パイプラインライフサイクル契約

> **ステータス: 仕様書** — このドキュメントは目標とする契約を定義する。実装修正（`pipeline_next_action` での `sm.PhaseStart()` 呼び出しと `pipeline_report_result` での `phase-complete` イベント発行）は未完了。詳細は[現在のギャップ](#現在のギャップ修正待ち)を参照。

## 目的

このドキュメントは、パイプラインフェーズの**必須状態遷移契約**を定義する。すべてのフェーズは対称的な `PhaseStart` / `PhaseComplete` ライフサイクルに従わなければならない。どのコンポーネントもこの契約を迂回してはならない。

## この契約が解決する問題

パイプラインには2つの実行パスがある:

1. **スタンドアロンハンドラ** (`phase_start`, `phase_complete` MCPツール) — 適切な状態更新とイベント発行を伴い `sm.PhaseStart()` と `sm.PhaseComplete()` を呼び出す
2. **パイプラインエンジン** (`pipeline_next_action` + `pipeline_report_result`) — メインループを駆動するが、歴史的に `sm.PhaseStart()` を迂回していたため、以下の問題が発生:
   - `CurrentPhaseStatus` が `"in_progress"` ではなく `"pending"` のまま
   - `Timestamps.PhaseStarted` が設定されない
   - `phase-start` イベントが発行されない（ダッシュボードに完了まで何も表示されない）
   - `phase-complete` イベントが `pipeline_report_result` から発行されない

この契約は、すべてのパスが同一のライフサイクルに従うことで不整合を排除する。

## フェーズライフサイクル

すべてのフェーズ遷移はこのシーケンスに従う。どのステップもスキップしてはならない。

```
pending ──[PhaseStart]──> in_progress ──[PhaseComplete]──> (次フェーズ: pending)
   |                           |
   |                           |──[PhaseFail]──> failed
   |                           └──[Checkpoint]──> awaiting_human
   |
   └──[PhaseCompleteSkipped]──> (次フェーズ: pending)
```

### 状態変更

| 遷移 | メソッド | `CurrentPhaseStatus` | `Timestamps.PhaseStarted` | イベント |
|---|---|---|---|---|
| **開始** | `sm.PhaseStart(workspace, phase)` | `"in_progress"` | `nowISO()` | `phase-start` |
| **完了** | `sm.PhaseComplete(workspace, phase)` | `"pending"` (次) or `"completed"` | `nil` | `phase-complete` |
| **失敗** | `sm.PhaseFail(workspace, msg)` | `"failed"` | _(変更なし)_ | `phase-fail` |
| **チェックポイント** | `sm.Checkpoint(workspace, phase, ...)` | `"awaiting_human"` | _(変更なし)_ | `checkpoint` |
| **スキップ** | `sm.PhaseCompleteSkipped(workspace, phase)` | `"pending"` (次) | `nil` | _(なし)_ |

**イベント発行に関する注記**: `sm.PhaseStart()` と `sm.PhaseComplete()` は純粋な状態変更であり、イベント自体は発行しない。呼び出し元が状態変更成功後に `publishEvent()` を呼ぶ責任を持つ。これにより `StateManager` は `EventBus` への依存を持たない（[設計判断](#イベントがハンドラレベルで発行される理由)参照）。

### 不変条件

1. **対称的な開始/完了**: すべての `PhaseStart` の後には、必ず1回の `PhaseComplete`、`PhaseFail`、または `Abandon` が続く。開始されていないフェーズを完了してはならない。
2. **単一書き込み者**: 1つのコンポーネントのみが特定のフェーズを遷移する。パイプラインループでは、`pipeline_next_action` が `PhaseStart` を、`pipeline_report_result` が `PhaseComplete` を所有する。
3. **イベントと状態の整合性**: イベントは対応する状態変更が成功した**後に**発行される。変更が失敗した場合、イベントは発行されない。
4. **冪等性**: `Engine.NextAction()` は読み取り専用 — 状態を変更しない。シグナルを返し、呼び出し元 (`pipeline_next_action`) が状態遷移を担当する。

## 実行パス

### パス 1: パイプラインエンジン（主要）

メイン実行ループ。`/forge` を介したすべての自動パイプライン実行で使用。

```
pipeline_next_action
  |-- eng.NextAction() -> Action        [読み取り専用の判断]
  |-- sm.PhaseStart(workspace, phase)   [状態: pending -> in_progress]
  |-- publishEvent("phase-start")       [ダッシュボード通知]
  └-- return Action to orchestrator     [オーケストレーターが実行]

[オーケストレーターがアクションを実行: Agent, exec, write_file]

pipeline_report_result
  |-- sm.PhaseLog(...)                  [メトリクス記録]
  |-- determineTransition()
  |   └-- sm.PhaseComplete(...)         [状態: in_progress -> pending (次)]
  |-- publishEvent("phase-complete")    [ダッシュボード通知]
  └-- return next_action_hint
```

**アクションタイプごとの動作:**

| アクションタイプ | `phase-start` 発行? | `agent-dispatch` 発行? | 報告方法 |
|---|---|---|---|
| `spawn_agent` | Yes | Yes (エージェント名付き) | `pipeline_report_result` |
| `exec` | Yes | No | `pipeline_report_result` (P5 埋め込みパス) |
| `write_file` | Yes | No | `pipeline_report_result` (P5 埋め込みパス) |
| `checkpoint` | No (パス3参照) | No | チェックポイントフロー |
| `done` (skip) | No | No | P1 スキップループ (内部) |

**P1 スキップループ**: `Engine.NextAction()` が `SkipSummaryPrefix` 付きの `ActionDone` を返した場合、`pipeline_next_action` は内部で吸収する — `sm.PhaseCompleteSkipped()` を呼び、上限20回のループ内で `eng.NextAction()` を再呼び出しする。スキップされたフェーズには `phase-start` や `phase-complete` イベントは発行されない。

### パス 2: スタンドアロンハンドラ（デバッグ / 手動）

個別の `phase_start` / `phase_complete` MCPツール。手動での状態操作やデバッグに使用。

```
PhaseStartHandler
  |-- ガードチェック (例: phase-5 でタスクが空でないこと)
  |-- sm.PhaseStart(workspace, phase)
  └-- publishEvent("phase-start")

PhaseCompleteHandler
  |-- ガードチェック (アーティファクト存在、awaiting_human でない、pending revision なし)
  |-- sm.PhaseComplete(workspace, phase)
  └-- publishEvent("phase-complete")
```

### パス 3: チェックポイントフロー

ヒューマンレビューゲート。`pipeline_next_action` はチェックポイントフェーズを検出し、`sm.Update()` を介して `awaiting_human` を設定する（`sm.Checkpoint()` ではない — スタンドアロンチェックポイントハンドラは別の MCP ツール）。オーケストレーターがユーザーにチェックポイントを提示し、レスポンスを返す。

```
pipeline_next_action (チェックポイントアクション検出)
  |-- sm.Update(): CurrentPhaseStatus = "awaiting_human"
  └-- return Action{type: "checkpoint"} to orchestrator

[ユーザーがレビューして応答]

pipeline_next_action (user_response 付き)
  |-- "proceed" -> sm.PhaseComplete(workspace, phase)
  |-- "revise"  -> sm.Update() で状態を巻き戻し
  └-- "abandon" -> sm.Abandon()
```

**注記**: スタンドアロンの `CheckpointHandler` (`handlers.go`) は `sm.Checkpoint()` を呼び `checkpoint` イベントを発行する。パイプラインエンジンパスは `sm.Update()` を直接使用する。両方とも `CurrentPhaseStatus = "awaiting_human"` となるが、異なるコードパスを通る。

## イベント分類

| イベント | 現在の発行者 | タイミング | 結果 |
|---|---|---|---|
| `pipeline-init` | `pipeline_init_with_context` | ワークスペース作成時 | `in_progress` |
| `phase-start` | `pipeline_next_action` (spawn_agent のみ) | フェーズ開始時 | `in_progress` |
| `agent-dispatch` | `pipeline_next_action` | エージェント起動時 | `dispatched` |
| `action-complete` | `pipeline_next_action` (P5 埋め込みレポートパス) | エージェント/exec完了時 | `completed` |
| `phase-complete` | `PhaseCompleteHandler` のみ | フェーズ完了時 | `completed` |
| `phase-fail` | `PhaseFailHandler` | フェーズ失敗時 | `failed` |
| `checkpoint` | `CheckpointHandler` のみ | ヒューマン待機時 | `awaiting_human` |
| `revision-required` | `pipeline_next_action` | レビューREVISE判定時 | `failed` |
| `pipeline-complete` | `pipeline_next_action` | 全フェーズ完了時 | `completed` |
| `abandon` | `AbandonHandler` | パイプライン放棄時 | `abandoned` |

### フェーズごとの期待イベントシーケンス（目標）

契約が完全に実装された後、通常の `spawn_agent` フェーズは以下を生成する:

```
phase-start (in_progress)
  -> agent-dispatch (dispatched)
  -> action-complete (completed)
  -> phase-complete (completed)
```

`exec` または `write_file` フェーズは以下を生成する:

```
phase-start (in_progress)
  -> action-complete (completed)
  -> phase-complete (completed)
```

## 現在のギャップ（修正待ち）

この仕様と現在の実装の間に以下のギャップが存在する:

| ギャップ | 場所 | 必要な修正 |
|---|---|---|
| `sm.PhaseStart()` が呼ばれない | `pipeline_next_action.go` ~line 453 | `publishEvent("phase-start")` の前に `sm2.PhaseStart(workspace, action.Phase)` を追加 |
| `phase-start` が `spawn_agent` のみで発行 | `pipeline_next_action.go` ~line 452 switch | `exec` と `write_file` アクションでも発行する |
| `phase-complete` が発行されない | `pipeline_report_result.go` / `phase_transition.go` | `determineTransition()` 内の `sm.PhaseComplete()` の後に `publishEvent("phase-complete")` を追加 |
| `checkpoint` イベントがエンジンパスで発行されない | `pipeline_next_action.go` ~line 427 | `sm.Update()` で `awaiting_human` 設定後に `publishEvent("checkpoint")` を追加 |

## 設計判断

### PhaseStart が pipeline_next_action に置かれる理由

`pipeline_next_action` はパイプラインループにおけるフェーズ遷移の単一エントリポイントである。ここに `PhaseStart` を配置することで:

- **局所性**: 開始遷移がディスパッチ判断の隣にあり、コードの監査が容易
- **対称性**: `pipeline_next_action` がフェーズを開始し、`pipeline_report_result` が完了する
- **エンジンの純粋性**: `Engine.NextAction()` は状態の純粋関数のまま — 副作用なし
- **レイヤー準拠**: `tools -> orchestrator -> state` のインポート方向が保持される

### スタンドアロンハンドラが残される理由

`phase_start` と `phase_complete` MCPツールは以下の用途で利用可能:

- 中断後の手動状態復旧
- 開発中のパイプライン状態デバッグ
- パイプラインループ外で動作する将来のCLIツール

同一の契約に従い、パイプラインエンジンパスと競合してはならない。

### イベントがハンドラレベルで発行される理由

`StateManager` は外部依存のない純粋な状態永続化レイヤーである。`EventBus` を追加すると `tools -> orchestrator -> state` レイヤリングに違反する:

```
tools (publishEvent + sm.PhaseStart)
  -> orchestrator (Engine — 読み取り専用)
    -> state (StateManager — 永続化のみ)
```

イベントは**プレゼンテーション関心事**（ダッシュボード、Slack）であり、バスが利用可能なハンドラレベルに属する。
