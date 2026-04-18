# ダッシュボードのリモートコントロール

Status: draft v1 (2026-04-18)

## 概要

このドキュメントでは、forge ダッシュボードを外部デバイス（スマートフォン、タブレット、リモートマシン）
からアクセス可能にし、チェックポイントの承認が Claude パイプラインを自動的に再開できるようにするための
アーキテクチャについて説明します。ターミナルへの入力は不要です。

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

ユーザーがダッシュボードの "approve" をクリックすると、`sm.PhaseComplete()` が
`state.json` を次のフェーズに進めます。しかし Claude は `AskUserQuestion` でターミナル
入力を待ったままブロックされています。承認はステートに書き込まれますが、Claude は
目覚めません。

また、ダッシュボードサーバーは `127.0.0.1` にのみバインドされているため、外部デバイスから
のアクセスは変更なしでは不可能です。

### 目標

1. ダッシュボードの承認でパイプラインが自動再開される — ターミナル操作は不要。
2. ngrok 経由でダッシュボードを外部デバイス（スマートフォン、リモートマシン）からアクセス可能にする。
3. マルチパイプライン監視とタスク投入（フェーズ2）の基盤を構築する。

---

## フェーズ1: EventBus ロングポール + リモートアクセス

### コアメカニズム: `pipeline_next_action` における EventBus ロングポール

SKILL.md がチェックポイントで `AskUserQuestion` をブロックする代わりに、MCP ツール
自体がステートの変化を待機します。`pipeline_next_action` が `currentPhaseStatus ==
"awaiting_human"` かつ `user_response` なしで呼ばれた場合:

1. ハンドラーはプロセス内の `EventBus` を購読します。
2. 現在のチェックポイントフェーズの `phase-complete` イベントを最大 **N 秒**（例: 15 秒
   — MCP ツールコールタイムアウト内で安全）待機します。
3. **イベントが届いた場合**（ダッシュボードが `PhaseComplete` を呼出し → EventBus が発行）:
   ステートを再読込 → `eng.NextAction` → `sm.PhaseStart` → 次の実際のアクション
   （例: フェーズ4の `spawn_agent`）を返す。Claude はターミナル操作なしで続行します。
4. **タイムアウト**（イベントなし）: `{type: "checkpoint", still_waiting: true}` と同じ
   チェックポイント表示テキストを返します。

```text
Claude（AskUserQuestion なし）     MCP サーバー                ダッシュボード
       │                                │                           │
       │── pipeline_next_action() ─────▶│                           │
       │                                │ EventBus を購読           │
       │   [MCP コール ブロック中]      │ 最大15秒待機              │
       │                                │                           │
       │                                │◀── PhaseComplete() ───────│ ユーザーが承認クリック
       │                                │ イベント: phase-complete   │
       │                                │ ステート再読込            │
       │                                │ eng.NextAction            │
       │◀─ {type: "spawn_agent"} ───────│ PhaseStart               │
       │                                │                           │
       │  パイプライン続行              │                           │
```

15 秒以内にダッシュボードの承認がなければ、ツールは `still_waiting: true` を返します。
SKILL.md は即座に `pipeline_next_action` を再度呼びます（スリープなし、ターミナルプロンプト
なし）。15 秒のサーバーサイド遅延が自然なペーシングを提供します。

### ターミナルユーザーのパス

最大 15 秒の遅延はありますが、ターミナルの正確性は影響を受けません:

1. Claude は `pipeline_next_action` ロングポールでブロックされています。
2. ユーザーがターミナルで "proceed" と入力 → Claude Code はメッセージをキューに追加。
3. 15 秒後（またはダッシュボードが先に承認した場合）、ツールは `still_waiting: true` を返します。
4. Claude はキューの "proceed" を処理 → `pipeline_next_action(user_response="proceed")` を呼出し。
5. P8 ブロックが `sm.PhaseComplete` を呼び、エンジンが次のアクションを返します。

### SKILL.md の変更（最小限）

変更はチェックポイントアクションのハンドラーのみです:

```text
- `checkpoint`: checkpoint() を呼出し。action.present_to_user + ダッシュボード URL を出力。
  次に即座に pipeline_next_action() を呼出す（user_response なし、AskUserQuestion なし）。
  - still_waiting: true  → 即座に pipeline_next_action() を再呼出し。
  - チェックポイント以外のアクションが返った → ダッシュボードが承認済み; 通常通り続行。
  - ユーザーはいつでもターミナルで入力可能; メッセージは現在の pipeline_next_action()
    呼出しが返った後（最大15秒）に処理される。
```

### 変更サマリー

| コンポーネント | 変更内容 | 規模 |
| --- | --- | --- |
| `pipeline_next_action.go` | `awaiting_human` + `user_response` なし時のロングポール追加 | 中 |
| `SKILL.md` | `AskUserQuestion` を `still_waiting` での即時再呼出しに置き換え | 小 |
| `dashboard.html` | 承認後: "Pipeline will continue automatically" を表示 | 小 |
| `intervention.go` | オプション: ベアラートークン認証（`FORGE_DASHBOARD_TOKEN`） | 小 |

### ngrok 経由のリモートアクセス

サーバーのバインドアドレス変更は不要です。ダッシュボードは `127.0.0.1` のままです。
ngrok が外部トラフィックをローカルポートに転送します:

```text
スマートフォンブラウザ
       │
       ▼ HTTPS
  ngrok トンネル（例: https://abc123.ngrok.io）
       │
       ▼ HTTP（ループバック）
  127.0.0.1:8099  ←  forge ダッシュボードサーバー
```

`intervention.go` の `isLocalRequest` ガードは ngrok エージェントのループバック
接続のみを見るため、変更なしで通過します。

ブラウザからの `Origin` ヘッダーは ngrok の URL になるため、現在のオリジンチェックは
失敗します。修正: `FORGE_DASHBOARD_TOKEN` が設定されており、リクエストに有効な
`Authorization: Bearer <token>` ヘッダーがある場合、オリジンチェックをバイパスします
（トークン認証が CSRF 保護に取って代わります）。

**セキュリティモデル:**

| モード | バインド | 認証 |
| --- | --- | --- |
| デフォルト（現在） | `127.0.0.1` | ループバック + 同一オリジン（変更なし） |
| リモート（ngrok） | `127.0.0.1` | `FORGE_DASHBOARD_TOKEN` ベアラートークン |

ngrok の設定例:

```bash
# ダッシュボード付きで MCP サーバーを起動
FORGE_EVENTS_PORT=8099 FORGE_DASHBOARD_TOKEN=<secret> forge-state-mcp

# 別のターミナルで ngrok 経由で公開
ngrok http 8099 --request-header-add "Authorization: Bearer <secret>"
```

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

1. **`pipeline_next_action.go`**: `currentPhaseStatus == "awaiting_human"` かつ
   `user_response` なしの場合に EventBus ロングポールを追加。
   - `bus`（すでにハンドラーに渡されている）を購読。
   - EventBus チャネル（現在のフェーズの `phase-complete`）、15 秒ティッカー、
     `ctx.Done()` を select。
   - マッチした場合: ステート再読込、エンジン実行、`PhaseStart` 呼出し、アクション返却。
   - タイムアウト: `still_waiting: true` のチェックポイントアクションを返却。

2. **`SKILL.md`**: チェックポイントアクションハンドラー — `AskUserQuestion` を削除し、
   `still_waiting: true` での即時再呼出しループを追加。

3. **`intervention.go`**: オプションのベアラートークンミドルウェアを追加。
   - サーバー起動時に環境変数から `FORGE_DASHBOARD_TOKEN` を読込。
   - 設定済みの場合: `Authorization: Bearer <token>` が一致すれば任意のオリジンを許可;
     不一致の場合は拒否（ループバックに関わらず）。
   - 未設定の場合: 既存の `isLocalRequest` の動作は変更なし。

4. **`dashboard.html`**: 承認後の UX — "approved" ボタンラベルを
   "✓ Pipeline will continue automatically" に変更。

### フェーズ2（将来）

Agent SDK ベースのタスクランナーとダッシュボード投入フォームを独立した取り組みとして
設計・実装します。
