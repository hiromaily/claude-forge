# ダッシュボードのリモートコントロール

Status: draft v3 (2026-04-19)

## 概要

このドキュメントでは、forge ダッシュボードを外部デバイス（スマートフォン、タブレット、リモートマシン）
からアクセス可能にし、チェックポイントの承認が Claude パイプラインを自動的に再開できるようにするための
アーキテクチャについて説明します。ターミナルへの入力は不要です。

セキュリティ強化は後のフェーズに先送りします。このドキュメントの設計は初期開発・ドッグフーディング
フェーズのみを対象としています。

## 問題の概要

### 現在のフロー

```text
Claude（ターミナル）         ダッシュボード（ブラウザ）      state.json
       │                          │                         │
       │── pipeline_next_action ──▶ エンジン: checkpoint    │
       │                          │                         │
       │── checkpoint() ──────────────────────────────────▶ │ status=awaiting_human
       │                          │                         │
       │  [AskUserQuestion]       │── approve ボタン ──────▶│ PhaseComplete()
       │  ターミナル入力待ち       │                         │ status=pending（次フェーズ）
       │  でブロック中             │   "approved" ✓          │
       │                          │                         │
       │  [何も起こらない]         │                         │
       │  まだブロック中           │                         │
```

ユーザーがダッシュボードの "approve" をクリックすると、`approveCheckpointHandler` が
`sm.PhaseComplete()` を呼び出し、`state.json` を次のフェーズに進めます。しかし:

1. **EventBus にイベントが発行されない** — `approveCheckpointHandler` は `bus` に
   アクセスできないため、EventBus は承認を通知されません。
2. **Claude は `AskUserQuestion` でターミナル入力を待ったままブロック**されています。
   イベントが発行されたとしても、受信する側がいません。

また、ダッシュボードサーバーは `127.0.0.1` にのみバインドされているため、
外部デバイスからのアクセスは不可能です。

### すでに実装済みのもの

- **`checkpoint-message.txt` 注入**: ユーザーがダッシュボードからメッセージ付きで
  チェックポイントを承認すると、メッセージがワークスペースの `checkpoint-message.txt`
  に書き込まれます。`pipeline_next_action.go` の `enrichPrompt` がこのファイルを
  読み込んで削除し、次のエージェントのプロンプトに自動的に注入します。
- **`currentPhaseStatus = "awaiting_human"` の直接設定**: `pipeline_next_action`
  が `ActionCheckpoint` を返す際に直接設定されるようになり、チェックポイントアクションと
  `checkpoint()` MCP 呼び出しの間のウィンドウが解消されました。
- **EventBus + SSE**: イベントバスは稼働しており、ダッシュボードの SSE ストリームは
  機能しています。不足しているのは `approveCheckpointHandler` がそこに発行しないことです。

### 目標

1. ダッシュボードの承認でパイプラインが自動再開される — ターミナル操作は不要。
2. 同一ネットワーク上の外部デバイス（スマートフォン、タブレット）から
   `0.0.0.0` バインドモードでダッシュボードにアクセス可能にする。
3. マルチパイプライン監視とタスク投入（フェーズ2）の基盤を構築する。

---

## フェーズ1: EventBus ロングポール + ローカルネットワークアクセス

### コアメカニズム: `pipeline_next_action` における EventBus ロングポール

SKILL.md がチェックポイントで `AskUserQuestion` をブロックする代わりに、MCP ツール
自体がステートの変化を待機します。`pipeline_next_action` が `currentPhaseStatus ==
"awaiting_human"` かつ `user_response` なしで呼ばれた場合:

1. ハンドラーはプロセス内の `EventBus` を購読します。
2. 現在のチェックポイントフェーズの `phase-complete` イベントを最大 **15 秒**
   （MCP ツールコールタイムアウト内で安全）待機します。
3. **イベントが届いた場合**（ダッシュボードが `PhaseComplete` を呼出し → EventBus が発行）:
   ステートを再読込 → `eng.NextAction` → `sm.PhaseStart` → 次の実際のアクション
   （例: フェーズ4の `spawn_agent`）を返す。Claude はターミナル操作なしで続行します。
4. **タイムアウト**（イベントなし）: `eng.NextAction` を実行（同じチェックポイントアクションを返す）し、
   レスポンスに `still_waiting: true` を設定します。SKILL.md は即座に
   `pipeline_next_action()` を再度呼びます。

```text
Claude（AskUserQuestion なし）     MCP サーバー                ダッシュボード
       │                                │                           │
       │── pipeline_next_action() ─────▶│                           │
       │                                │ EventBus を購読           │
       │   [MCP コール ~15秒ブロック]   │ 最大15秒待機              │
       │                                │                           │
       │                                │◀── PhaseComplete() ───────│ ユーザーが承認クリック
       │                                │ イベント: phase-complete   │
       │                                │ ステート再読込            │
       │                                │ eng.NextAction            │
       │◀─ {type: "spawn_agent"} ───────│ PhaseStart               │
       │                                │                           │
       │  パイプライン続行              │                           │
```

タイムアウト時、`pipeline_next_action` は `{type: "checkpoint", still_waiting: true}` を返します。
SKILL.md は即座に `pipeline_next_action` を再度呼びます（スリープなし）。15 秒の
サーバーサイド遅延が自然なペーシングを提供します。

### ターミナルユーザーのパス

最大 15 秒の遅延はありますが、ターミナルの正確性は影響を受けません:

1. Claude は `pipeline_next_action` ロングポールでブロックされています。
2. ユーザーがターミナルで "proceed" と入力 → Claude Code はメッセージをキューに追加。
3. 15 秒後（またはダッシュボードが先に承認した場合）、ツールが返ります — 次のアクション
   （ダッシュボード承認済み）または `still_waiting: true`（タイムアウト）のどちらかで。
4. `still_waiting` の場合、Claude はキューの "proceed" を処理 →
   `pipeline_next_action(user_response="proceed")` を呼出し。
5. P8 ブロックが `sm.PhaseComplete` を呼び、エンジンが次のアクションを返します。

### 不足しているリンク: `approveCheckpointHandler` に bus が接続されていない

`approveCheckpointHandler` は現在 `*state.StateManager` のみを受け取ります。
`sm.PhaseComplete()` 呼出し後、ロングポールが起きるように `phase-complete`
イベントを発行する必要があります:

```go
// sm.PhaseComplete 成功後:
bus.Publish(events.Event{
    Event:     "phase-complete",
    Phase:     req.Phase,
    Workspace: req.Workspace,
    Outcome:   "completed",
    Timestamp: time.Now().UTC().Format(time.RFC3339),
})
```

`server.go` は `bus` を `approveCheckpointHandler` に渡す必要があります。

### SKILL.md の変更（最小限）

変更はチェックポイントアクションのハンドラーのみです:

```text
- `checkpoint`:
  1. checkpoint(workspace, phase=action.name) を呼出してポーズを登録。
  2. action.present_to_user をユーザーに提示し、ターミナル入力なしに
     ダッシュボードから承認できることを伝える。
  3. 即座に pipeline_next_action(workspace) を呼出す（user_response なし、
     previous_* なし）。still_waiting: true なら再呼出し。チェックポイント以外の
     アクションが返るまで繰り返す。
  4. ユーザーがターミナルで入力（proceed/revise/abandon）した場合: 15 秒の
     ロングポール中にメッセージがキューに入る。次の pipeline_next_action 呼出し時に
     ループではなく user_response=<message> を渡す。
```

### 変更サマリー

| コンポーネント | 変更内容 | 規模 |
| --- | --- | --- |
| `dashboard/server.go` | `bus` を `approveCheckpointHandler` に渡す | 微小 |
| `dashboard/intervention.go` | `bus` パラメータ追加; `PhaseComplete` 後に `phase-complete` を発行 | 小 |
| `handler/tools/pipeline_next_action.go` | `awaiting_human` + `user_response` なし時のロングポール追加 | 中 |
| `skills/forge/SKILL.md` | `AskUserQuestion` を `still_waiting` での即時再呼出しに置き換え | 小 |
| `nextActionResponse` | `StillWaiting bool` フィールドを追加 | 微小 |

### `FORGE_DASHBOARD_BIND_ALL` によるローカルネットワークアクセス

初期開発フェーズでは認証も ngrok も不要です。`FORGE_DASHBOARD_BIND_ALL=1` を
追加すると、ダッシュボードが `127.0.0.1` の代わりに `0.0.0.0` にバインドし、
`isLocalRequest` オリジンチェックを無効にします:

```text
スマートフォンブラウザ（同一 WiFi）
       │
       ▼ HTTP
  192.168.x.x:8099  ←  forge ダッシュボードサーバー（0.0.0.0:8099）
```

実装:
- `server.go`: 起動時に `FORGE_DASHBOARD_BIND_ALL` を読込; 設定済みなら `0.0.0.0`
  を使用、それ以外は `127.0.0.1` を維持。
- `intervention.go`: `FORGE_DASHBOARD_BIND_ALL` 設定時は `isLocalRequest` チェックを
  スキップ。`server.go` からハンドラーに `publicMode bool` フラグを渡す。

パブリックモードでのダッシュボード起動:

```bash
FORGE_EVENTS_PORT=8099 FORGE_DASHBOARD_BIND_ALL=1 forge-state-mcp
```

その後、同一ネットワーク上の任意のデバイスから `http://<ホストIP>:8099` を開く。

**セキュリティ注意**: これは意図的に安全でなく、ローカル開発のみを対象としています。
同一ネットワーク上の誰でもチェックポイントを承認してパイプラインを放棄できます。
ベアラートークン認証と ngrok サポートは将来のフェーズに先送りします。

---

## フェーズ2: Web UI からのタスク投入

### 3.1 エグゼクティブサマリー

フェーズ2により、ダッシュボード Web UI から forge パイプラインタスクを投入できるようになります。
MCP サーバープロセスに組み込まれたタスクランナーが、投入された各タスクに対して Anthropic
Agent SDK セッションを起動します。Agent SDK セッションは、完全なマルチターン会話と
ツール使用サポートを備えた forge パイプラインを実行します。セッションは同じ `.specs/`
ワークスペースツリーに書き込み、同じプロセス内の `EventBus` に発行するため、ダッシュボードの
SSE ストリームとチェックポイント承認フローは、インタラクティブ（Claude Code）と SDK 実行の
両パイプラインで同一に動作します — コントロールプレーンは統一されます。

`claude --print`（`-p`）はステートレスであり、マルチターンのパイプライン会話には適して
いません。適切なツールは **Anthropic Agent SDK** です。これはプログラム的に完全な
ツール使用付きのマルチターン会話をサポートします。

### 3.2 HTTP API

```
POST /api/task/submit
Authorization: Bearer <token>   （FORGE_DASHBOARD_TOKEN が設定されている場合に必須）
Content-Type: application/json

{
  "input":  "https://github.com/org/repo/issues/42",
  "effort": "M",
  "flags":  ["--auto"]
}
```

レスポンス（202 Accepted）:
```json
{
  "task_id": "20260419-42-fix-login-timeout",
  "status":  "queued"
}
```

```
GET /api/tasks
Authorization: Bearer <token>   （FORGE_DASHBOARD_TOKEN が設定されている場合に必須）
```

レスポンス（200 OK）:
```json
{
  "tasks": [
    {
      "task_id":    "20260419-42-fix-login-timeout",
      "input":      "https://github.com/org/repo/issues/42",
      "status":     "running",
      "workspace":  ".specs/20260419-42-fix-login-timeout",
      "queued_at":  "2026-04-19T10:30:00Z",
      "started_at": "2026-04-19T10:30:05Z"
    }
  ]
}
```

**バリデーション**: `input` フィールドは既存の `handler/validation.ValidateInput` 関数で
検証されます（`pipeline_init` と同じパス）。`effort` は `S`、`M`、`L` のいずれか
（省略可、省略時は forge が自動選択）。`flags` エントリは初期実装では `["--auto"]` のみ
許可されます。

**デコーダー**: 専用の `json.NewDecoder` を持つ新しい `taskSubmitRequest` 構造体を使用します
（`intervention.go` の `decodeRequest` は `DisallowUnknownFields` を使用し、ボディ形状が
異なるため使用しません）。新しいデコーダーは同じ
`http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)` パターンに従います。

### 3.3 Go パッケージレイアウト

```text
mcp-server/internal/taskrunner/
  runner.go          — Runner 構造体: ゴルーチンプール、タスクキュー、ライフサイクル
  task.go            — Task 構造体: ID、input、effort、flags、status、タイムスタンプ
  queue.go           — インメモリキュー + tasks.json 永続化
mcp-server/internal/dashboard/
  task_submit.go     — POST /api/task/submit ハンドラー
  task_list.go       — GET /api/tasks ハンドラー
```

**依存関係の方向**（インポート DAG `tools → orchestrator → state` に準拠）:

```text
dashboard/task_submit.go → taskrunner（エンキューのみ）
taskrunner/runner.go     → engine/state（結果確認のための ReadState、PhaseComplete は不使用）
taskrunner/queue.go      → engine/state（再開スキャンのための ReadState のみ）
```

`taskrunner` は `handler/tools` または `engine/orchestrator` をインポートしてはなりません。
`taskrunner` はタスクの結果を確認するためにのみ `state.json` を読み取ります
（`queue-design.md` の `queue_report` と同じパターン）。

### 3.4 `StartOptions` の拡張

`StartOptions`（`mcp-server/internal/dashboard/server.go` で定義）に `TaskRunner`
フィールドが追加されます:

```go
type StartOptions struct {
    PhaseLabels map[string]string
    TaskRunner  *taskrunner.Runner   // nil → タスク投入エンドポイントは 501 を返す
}
```

`Start` 関数は `opts.TaskRunner != nil` の場合に `POST /api/task/submit` と
`GET /api/tasks` を登録します。nil の場合、ルートは登録されますが `501 Not Implemented`
を返します（ランナーの起動失敗時の nil 参照パニックを回避）。

これは既存の `*StartOptions` パターンを拡張するもので、`Start` のシグネチャは変更しません。

### 3.5 Agent SDK ランタイムオプション

フェーズ2実装パイプラインは、SDK の利用可能状況に基づいてランタイムを選択する必要があります。
優先順位順に3つのオプションを示します:

1. **Go Anthropic SDK**（推奨）: MCP サーバーと同一プロセスで Agent セッションを保持し、
   クロスランゲージの依存関係を回避。実装時に Go Anthropic SDK がマルチターン会話と
   ツール使用をサポートしている場合に使用。
2. **Node.js サブプロセス**: `@anthropic-ai/sdk` パッケージを使用する Node.js プロセスを
   `taskrunner.Runner` が起動。サブプロセスは stdin JSON でタスクを受け取り、進捗イベントを
   stdout に書き出す。Node.js ランタイム依存関係が追加される。
3. **Python サブプロセス**: `anthropic` Python パッケージを使用したオプション2と同じパターン。
   Go SDK も Node.js SDK も適切でない場合のフォールバック。サブプロセスを使用する場合は、
   `os/exec` 呼び出しに `//nolint:gosec // G204` を注記してください（`.golangci.yml` は
   すでに G204 を抑制しています）。

HTTP API コントラクト（`POST /api/task/submit`、`GET /api/tasks`、`tasks.json` 永続化）は
ランタイムに依存しません。内部の Agent セッション起動メカニズムのみが SDK の選択によって変わります。

### 3.6 `artifactHandler` パブリックモード修正（フェーズ2の前提条件）

`mcp-server/internal/dashboard/artifact.go` は現在 `publicMode` を無視し、直接
`isLocalRequest(r)` を呼び出しており（29行目）、パブリックモードでも外部デバイスが
アーティファクトを参照できません。

必要な修正（このドキュメントパイプラインでは実装しません）:

```go
// 現在（パブリックモードで不正）:
if !isLocalRequest(r) {

// 修正後:
if !publicMode && !isLocalRequest(r) {
```

`artifactHandler` は `publicMode bool` パラメータを受け取る必要があります（クロージャ経由で
追加 — `approveCheckpointHandler` や `abandonHandler` と同じコンストラクタパターン）。
`server.go` は `artifactHandler(public)` として登録します。

これはフェーズ2の前提条件です: 外部デバイスがアーティファクト `.md` ファイル（design.md、
tasks.md）を取得できることが、リモートダッシュボードを有用にするために必要です。
**この Go の変更はこのドキュメントパイプラインでは実装されず**、フェーズ2の Go 実装
パイプラインで実施する必要があります。

### 3.7 タスクランナーのライフサイクル

**起動**: `Runner.Start(ctx context.Context)` は固定サイズのゴルーチンプール
（デフォルト: 1 ワーカー）を起動します。プールは `Enqueue` によって供給される
インメモリチャネルから読み取ります。

**クラッシュリカバリ**: `Runner.Start()` 時に、ランナーは `.specs/tasks.json` の
`status: queued` または `status: in_progress` のタスクをスキャンして再エンキューします。
ランナーは `source: "dashboard"` を持つタスクのみ再エンキューします — この識別子フィールドにより、
インタラクティブな Claude Code セッションで開始されたパイプラインを誤って再エンキューしないようにします。

**Agent セッション**: 各タスクは Agent SDK セッションを起動します。セッションは forge
パイプラインをインタラクティブに実行します（マルチターン、パイプライン全ライフサイクルにわたる
完全なツール使用）。セッションは `FORGE_EVENTS_PORT` にアクセスでき、同じマシンの `.specs/`
に書き込むため、そのパイプラインは同じプロセス内 EventBus と同じダッシュボード SSE ストリームに
イベントを発行します。正確な SDK 起動メカニズム（Go SDK、Node.js サブプロセス、Python
サブプロセス）は、その時点での SDK の利用可能状況に基づいてフェーズ2の Go 実装パイプラインに
委ねられます（§3.5 参照）。

**ワークスペーススラッグ**: 入力 URL からスラッグ導出ロジックを使用して事前生成されます
（URL からソース ID を抽出: GitHub はイシュー番号、Jira は小文字キー）。スラッグは
Agent SDK セッションに渡されるため、`pipeline_init_with_context` の `user_confirmation`
で `workspace_slug` を渡すことができます。これは `pipeline_init_with_context.go` の
既存の `applyWorkspaceSlug` パスを使用し、forge への変更はありません。

**結果判定**: セッション終了後、ランナーはワークスペースの `state.json` を直接読み取って
結果を判定します（MCP ツール呼び出しなし）。`queue_report` と同じ決定論的ルール:
`currentPhase == "completed"` → 成功、それ以外 → 失敗。

**永続化**: `.specs/` の `tasks.json` がタスクキューのステートを保持します。各ステート遷移後に
アトミックに書き込まれます（一時ファイルに書き込み + `os.Rename`）。`source: "dashboard"`
フィールドは HTTP 投入ハンドラーが常に書き込むため、リカバリスキャンがダッシュボードタスクと
インタラクティブパイプラインを区別できます。フォーマット:

```json
{
  "tasks": [
    {
      "task_id":     "20260419-42-fix-login-timeout",
      "input":       "https://github.com/org/repo/issues/42",
      "effort":      "M",
      "flags":       ["--auto"],
      "source":      "dashboard",
      "status":      "completed",
      "workspace":   ".specs/20260419-42-fix-login-timeout",
      "slug":        "42",
      "queued_at":   "2026-04-19T10:30:00Z",
      "started_at":  "2026-04-19T10:30:05Z",
      "finished_at": "2026-04-19T10:45:12Z"
    }
  ]
}
```

### 3.8 認証

**環境変数**: `FORGE_DASHBOARD_TOKEN`。設定されている（非空の）場合、すべての変更エンドポイント
（`POST /api/task/submit`、`POST /api/checkpoint/approve`、`POST /api/pipeline/abandon`）は
`Authorization: Bearer <token>` を必要とします。タイミング攻撃を防ぐため、トークン比較は
`crypto/subtle.ConstantTimeCompare` を使用します。

`FORGE_DASHBOARD_TOKEN` が設定されていない場合、動作はフェーズ1から変わりません
（`publicMode` がアクセスを制御）。`FORGE_DASHBOARD_TOKEN` が空のとき、トークン強制は
明示的に無効化され、ローカル開発でのオプトインを使いやすくします。

**フェーズ2実装パイプラインへの後方互換性注記**: `FORGE_DASHBOARD_TOKEN` 強制を既存の
フェーズ1エンドポイント（`POST /api/checkpoint/approve`、`POST /api/pipeline/abandon`）に
追加することは、`FORGE_DASHBOARD_BIND_ALL=1` を設定しているが `FORGE_DASHBOARD_TOKEN`
を設定していない既存のデプロイメントにとって破壊的変更です。実装はトークン強制をオプトインに
しなければなりません — `FORGE_DASHBOARD_TOKEN` が非空の場合のみ有効。トークンを無条件に
強制してはなりません。

### 3.9 ダッシュボード UI の変更

`dashboard.html`（現在 777 行、ゼロ依存）に以下が追加されます:

1. **タスク投入フォーム**（`publicMode` 有効時のみ表示）:
   - クライアントサイドで `publicMode` を検出するメカニズム（例: `GET /api/server-info`
     エンドポイント、またはサーブ時に HTML に埋め込まれた値）はフェーズ2の Go 実装
     パイプラインに委ねます。意図は `publicMode=true` のときのみフォームを表示することで、
     検出メカニズムには Go コードが必要であり、このドキュメントパイプラインのスコープ外です。
   - `input`（URL またはフリーテキスト）のテキスト入力
   - `effort`（S / M / L / Auto）のドロップダウン
   - 送信ボタン → `POST /api/task/submit`
   - 返された `task_id` とステータスを表示

2. **タスク一覧パネル**:
   - `GET /api/tasks` を10秒ごとにポーリング
   - カラム: タスク ID、入力、ステータス、開始時刻
   - 行をクリックするとフェーズタイムラインがそのワークスペースのイベントにフィルタリング

3. **マルチワークスペース SSE フィルタリング**:
   - 既存のタイムラインビューは SSE イベントデータの `workspace` でフィルタリング
   - タスク一覧からタスクを選択すると、そのワークスペースに一致するイベントのみが
     タイムラインに表示される

### 3.10 比較表: forge-queue vs フェーズ2

| 次元 | forge-queue | フェーズ2 ダッシュボード |
|---|---|---|
| 投入方法 | `queue.yaml` ファイル、`/forge-queue` スキル | `POST /api/task/submit` HTTP |
| 並列性 | 逐次（1タスクずつ） | 逐次（1ワーカー、拡張可） |
| 永続化 | `queue.yaml` | `.specs/tasks.json` |
| 入力タイプ | イシュー URL のみ（`--auto` 強制） | イシュー URL + フリーテキスト + フラグ |
| セッションランタイム | タスクごとに別 `claude -p`（ステートレス） | タスクごとに Agent SDK（マルチターン） |
| `claude -p` / SDK の理由 | バッチタスクのコンテキスト分離 | マルチターンパイプラインにはライブコンテキストが必要 |
| ワークスペーススラッグ | `queue_next` が事前生成 | `taskrunner` が事前生成 |
| 結果記録 | `queue_report` MCP ツール | `runner.go` が `state.json` を直接読取 |
| 監視 | CLI のみ | ダッシュボード SSE + タスク一覧 |

### テスト戦略

**`mcp-server/internal/taskrunner/` ユニットテスト**:
- `queue_test.go`: エンキュー/デキューのラウンドトリップ、`tasks.json` アトミック書き込み、
  重複 task_id の拒否、クラッシュリカバリスキャン（`source: "dashboard"` タスクのみ再エンキュー）
- `runner_test.go`: ワーカーゴルーチンがタスクを取得、Agent SDK セッションのライフサイクル、
  `state.json` からの結果判定（`completed` → 成功、それ以外 → 失敗）
- スラッグ生成: GitHub URL → イシュー番号、Jira URL → 小文字キー

**`mcp-server/internal/dashboard/` ハンドラーテスト**:
- `task_submit_test.go`: `input` を検証、未知の effort 値を拒否、`task_id` を含む 202 を返す、
  `FORGE_DASHBOARD_TOKEN` 設定時にトークンなしのリクエストを拒否、`TaskRunner` が未設定時に 501 を返す
- `task_list_test.go`: ランナーの現在のタスク一覧を返す、空リストを処理
- `artifact_test.go`（既存を拡張）: `publicMode=true` の `artifactHandler` がループバックチェック
  なしでアーティファクトを返す

**統合**（手動）:
- `POST /api/task/submit` で GitHub イシュー URL を投入し、起動されたパイプラインワークスペースの
  SSE イベントが表示されることを確認し、完了後に `tasks.json` が更新されることを確認

---

## 実装ロードマップ

### フェーズ1（今すぐ実装）

1. **`dashboard/server.go`**: `FORGE_DASHBOARD_BIND_ALL` 環境変数を読込; 設定済みなら
   `0.0.0.0` にバインドし、ハンドラーに `publicMode=true` を渡す。

2. **`dashboard/intervention.go`**: `bus *events.EventBus` と `publicMode bool` を
   ハンドラーコンストラクタに追加。
   - `publicMode` 時: `isLocalRequest` チェックをスキップ。
   - `sm.PhaseComplete()` 成功後: `bus` に `phase-complete` イベントを発行。

3. **`handler/tools/pipeline_next_action.go`**: P0 と `eng.NextAction` の間に
   ロングポールブロックを追加。`currentPhaseStatus == "awaiting_human"` かつ
   `user_response` なしの場合:
   - `bus` を購読し、select で: EventBus チャネル（現在フェーズの `phase-complete`）、
     15 秒タイマー、`ctx.Done()` を待機。
   - `phase-complete` 受信時: ステート再読込（`sm2.LoadFromFile`）し、`eng.NextAction`
     にフォールスルー。
   - タイムアウト/ctx 時: `eng.NextAction` にフォールスルー（同じチェックポイントアクションを返す）;
     `nextActionResponse` に `StillWaiting: true` を設定。

4. **`nextActionResponse`**: `StillWaiting bool \`json:"still_waiting,omitempty"\`` を追加。

5. **`skills/forge/SKILL.md`**: チェックポイントアクションハンドラー — `AskUserQuestion`
   を削除し、`still_waiting: true` での即時再呼出しループを追加。

### フェーズ2（将来）

§3.1–§3.10 で仕様化された Agent SDK ベースのタスクランナーとダッシュボード投入フォームを
実装します。主要な成果物:

1. **`mcp-server/internal/taskrunner/`**: `Runner`、`Task`、キュー永続化（`tasks.json`）を
   含む新パッケージ。実装時の Go SDK の利用可能状況に基づいて Agent SDK ランタイムを選択
   （Go SDK 推奨; Node.js または Python サブプロセスがフォールバック — §3.5 参照）。

2. **`dashboard/task_submit.go` + `dashboard/task_list.go`**: `opts.TaskRunner != nil`
   の場合に `POST /api/task/submit` と `GET /api/tasks` を登録（§3.2 と §3.4 参照）。

3. **`dashboard/artifact.go`**: 外部デバイスがアーティファクト `.md` ファイルを取得できる
   ように `publicMode bool` 修正を適用（前提条件 — §3.6 参照）。

4. **`dashboard/server.go`**: `TaskRunner` を `StartOptions` に組み込む; 変更エンドポイントの
   `FORGE_DASHBOARD_TOKEN` ベアラートークンミドルウェアを追加（§3.8 参照）。

5. **`dashboard.html`**: タスク投入フォームとタスク一覧パネルを追加（§3.9 参照）。

HTTP API コントラクト（`POST /api/task/submit`、`GET /api/tasks`、`tasks.json` 永続化フォーマット）
はこのドキュメントで固定されており、このリサーチドキュメントを先に更新しなければ変更してはなりません。
