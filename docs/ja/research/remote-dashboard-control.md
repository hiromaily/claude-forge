# ダッシュボードのリモートコントロール

Status: draft v2 (2026-04-19)

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

## フェーズ2: Web UI からのタスク投入（リサーチ）

Web UI からタスクを投入するにはプログラム的な Claude セッションが必要です。
`claude --print`（`-p`）はステートレスであり、マルチターンのパイプライン会話には
適していません。適切なツールは **Anthropic Agent SDK** です。これはプログラム的に
完全なツール使用付きのマルチターン会話をサポートします。

### アーキテクチャ

```text
Web UI  ──  POST /api/task/submit  ──▶  ダッシュボードサーバー
                                              │
                                        タスクをキューに追加
                                              │
                                        タスクランナー
                                        （Agent SDK）
                                              │
                                        マルチターンエージェントセッション
                                        forge パイプラインを実行
                                              │
                                        .specs/ ワークスペース
                                        state.json  ←──────  同じ EventBus
                                                             同じダッシュボード SSE
```

Agent SDK 実行パイプラインは同じ `state.json` に書き込み、同じ `EventBus` に発行する
ため、ダッシュボードの SSE ストリームとチェックポイント承認メカニズムは、インタラクティブ
（Claude Code）と SDK 実行の両パイプラインで同一に動作します。コントロールプレーンは
統一されます。

### タスク投入エンドポイント

```json
POST /api/task/submit
{
  "input":  "https://github.com/org/repo/issues/42",
  "effort": "M",
  "flags":  ["--auto"]
}
```

タスク ID を返します。タスクはダッシュボードに新しいパイプラインとして表示され、
同じ SSE + チェックポイントフローで監視・承認できます。

### forge-queue との統合

`forge-queue` 設計（`queue-design.md` 参照）は `claude -p` サブプロセスを通じた
逐次バッチ実行をカバーしています。フェーズ2はそのモデルを拡張し、ダッシュボード
駆動のタスク投入と SDK ベースの実行をサポートします:

- `forge-queue` → `claude -p` による逐次バッチ、ダッシュボード投入なし
- フェーズ2 → ダッシュボードからのオンデマンド投入、SDK ベース実行

これらは共存可能です。ダッシュボードのタスク投入エンドポイントは同じランナーのキューに
追加するだけです。

### フェーズ2に必要なもの

- Anthropic Agent SDK 統合（Python または TypeScript ランナー、または Go SDK）
- 永続的タスクランナーサービス（別プロセスまたはゴルーチンプール）
- タスクキューの永続化（シンプルなファイルベースまたはインメモリ）
- ダッシュボード: タスク投入フォーム + タスク一覧ビュー
- 認証強化（エンドポイントがタスク入力を受け付けるためトークンベース必須）

フェーズ2は独立した取り組みです。ここでは実装計画を提供しません。

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

Agent SDK ベースのタスクランナーとダッシュボード投入フォームを独立した取り組みとして
設計・実装します。
