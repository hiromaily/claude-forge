# MCPデータコントラクト

このドキュメントは、Claudeオーケストレーター（SKILL.md）とGo MCPサーバー（`forge-state-mcp`）間でパイプライン実行中にやりとりされる正確なJSONペイロードを規定します。以下の4つのツールがパイプライン全体のライフサイクルを駆動します。

> **信頼できるソース**: `mcp-server/internal/handler/tools/` および `mcp-server/internal/engine/orchestrator/actions.go` のGo構造体。スキーマ変更時は本ドキュメントとGoコードの両方を更新すること。

---

## 1. `pipeline_init` — 入力解析とレジューム検出

純粋な検出ツール。`/forge` の引数を解析し、ソースタイプを検出し、レジューム候補を確認します。**状態への副作用なし。**

### リクエスト

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|------|
| `arguments` | string | はい | `/forge` に渡された生の引数文字列 |
| `current_branch` | string | いいえ | `git branch --show-current` の出力 |

### レスポンス

**レジュームパス**（入力が既存の `.specs/` ディレクトリに一致）:

```json
{
  "resume_mode": "auto",
  "workspace": ".specs/20260330-fix-auth-timeout",
  "instruction": "call state_resume_info"
}
```

**新規パイプラインパス**:

```json
{
  "workspace": ".specs/20260401-https-github-com-owner-repo-issues-42",
  "spec_name": "https-github-com-owner-repo-issues-42",
  "source_type": "github_issue",
  "source_url": "https://github.com/owner/repo/issues/42",
  "source_id": "42",
  "core_text": "https://github.com/owner/repo/issues/42",
  "flags": {
    "auto": false,
    "skip_pr": false,
    "debug": false,
    "discuss": false,
    "effort_override": null,
    "current_branch": "main"
  },
  "fetch_needed": {
    "type": "github",
    "fields": ["labels", "title", "body"],
    "instruction": "fetch github issue fields before calling pipeline_init_with_context"
  }
}
```

**エラーパス**（無効な入力）:

```json
{
  "errors": ["input too short: minimum 3 characters required"]
}
```

### `source_type` 値

| 値 | トリガー |
|-------|---------|
| `github_issue` | `github.com/.../issues/\d+` に一致するURL |
| `jira_issue` | `*.atlassian.net/browse/...` に一致するURL |
| `text` | プレーンテキスト（デフォルト） |
| `workspace` | 入力に `.specs/` を含む |

---

## 2. `pipeline_init_with_context` — 3回コール確認フロー

複数回コールのハンドシェイクを実装: エフォート検出 → （オプション: ディスカッション） → 確認 & ワークスペース初期化。

### リクエスト

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|------|
| `workspace` | string | はい | `pipeline_init` からのワークスペースパス |
| `source_id` | string | いいえ | ソース識別子（例: `"42"`, `"SOA-123"`） |
| `source_url` | string | いいえ | 元のURL（GitHub/Jira） |
| `external_context` | object | いいえ | フェッチ済みのGitHub/Jiraフィールド |
| `flags` | object | いいえ | `pipeline_init` からのパース済みフラグ |
| `task_text` | string | いいえ | 元のタスクテキスト（テキストソースのみ） |
| `user_confirmation` | object | いいえ | 確認済みエフォート＋ブランチ決定（2回目コール） |
| `discussion_answers` | string | いいえ | ディスカッション質問へのユーザー回答 |

**`external_context` オブジェクト:**

```json
{
  "github_labels": ["bug", "priority-high"],
  "github_title": "Fix auth timeout in middleware",
  "github_body": "requests timeout after 30s",
  "jira_issue_type": "Bug",
  "jira_story_points": 3,
  "jira_summary": "Skip minutes job without integration",
  "jira_description": "..."
}
```

**`user_confirmation` オブジェクト（確認コール）:**

```json
{
  "effort": "M",
  "workspace_slug": "fix-auth-timeout",
  "use_current_branch": false,
  "enriched_request_body": "..."
}
```

### レスポンス — 1回目コール（エフォート検出）

オーケストレーターがユーザーに提示するための `needs_user_confirmation` を返す:

```json
{
  "needs_user_confirmation": {
    "detected_effort": "M",
    "effort_options": {
      "S": {
        "skipped_phases": [
          { "phase_id": "phase-2", "label": "Investigation" },
          { "phase_id": "phase-3b", "label": "Design Review" }
        ],
        "recommended": false
      },
      "M": {
        "skipped_phases": [
          { "phase_id": "phase-4b", "label": "Tasks Review" },
          { "phase_id": "checkpoint-b", "label": "Human Reviews Tasks" }
        ],
        "recommended": true
      },
      "L": {
        "skipped_phases": [],
        "recommended": false
      }
    },
    "current_branch": "main",
    "is_main_branch": true,
    "enriched_request_body": "implement login feature",
    "message": "Detected effort=\"M\". ..."
  }
}
```

### レスポンス — `--discuss` 付き1回目コール（テキストソースのみ）

```json
{
  "needs_discussion": {
    "questions": [
      "What is the main goal of this change?",
      "Are there any constraints or dependencies?",
      "What is the expected scope of changes?"
    ],
    "message": "Please answer the following questions..."
  }
}
```

### レスポンス — 確認コール（ワークスペース確定）

```json
{
  "ready": true,
  "workspace": ".specs/20260401-42-fix-auth-timeout",
  "effort": "M",
  "flow_template": "standard",
  "skipped_phases": ["phase-4b", "checkpoint-b"],
  "request_md_content": "---\nsource_type: github_issue\n...",
  "branch": "feature/42-fix-auth-timeout",
  "create_branch": true
}
```

### コール判別

| `discussion_answers` | `user_confirmation` | パス |
|---|---|---|
| なし | なし | 1回目コール → エフォート検出 |
| あり | なし | ディスカッションコール → 本文エンリッチ |
| なし | あり | 確認コール → ワークスペース初期化 |
| あり | あり | **エラー** — 曖昧 |

---

## 3. `pipeline_next_action` — アクションディスパッチ

コアループのドライバー。`state.json` を読み、`Engine.NextAction()` を決定論的に実行し、オーケストレーターが実行すべき型付きアクションを返す。

### リクエスト

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|------|
| `workspace` | string | はい | ワークスペースパス |
| `previous_action_complete` | boolean | いいえ | agent/exec/write_file完了後にtrue |
| `previous_tokens` | number | いいえ | 前回アクションのトークン数 |
| `previous_duration_ms` | number | いいえ | 前回アクションの所要時間（ms） |
| `previous_model` | string | いいえ | 前回アクションで使用したモデル |
| `previous_setup_only` | boolean | いいえ | 前回execがsetup-onlyの場合true |
| `user_response` | string | いいえ | チェックポイントアクションへのユーザー応答 |

### レスポンス構造

すべてのレスポンスは `Action` をオプションのメタデータでラップ:

```json
{
  "type": "spawn_agent",
  "warning": "",
  "display_message": "Phase 1: Situation Analysis",
  "report_result": null,
  ...アクション固有フィールド...
}
```

### アクション型

#### `spawn_agent` — LLMサブエージェントのディスパッチ

```json
{
  "type": "spawn_agent",
  "agent": "situation-analyst",
  "prompt": "...4層組み立てプロンプト...",
  "model": "sonnet",
  "phase": "phase-1",
  "input_files": ["request.md"],
  "output_file": "analysis.md",
  "parallel_task_ids": null
}
```

`prompt` フィールドは **4層組み立てプロンプト**（後述）を含む。`parallel_task_ids` が空でない場合、オーケストレーターはタスクIDごとに1つのエージェントを並行起動する。

#### `checkpoint` — 人間レビューのための一時停止

```json
{
  "type": "checkpoint",
  "name": "checkpoint-a",
  "present_to_user": "## Design Review\n\n...",
  "options": ["approve", "reject"]
}
```

#### `exec` — シェルコマンドの実行

```json
{
  "type": "exec",
  "phase": "pr-creation",
  "commands": ["gh", "pr", "create", "--title", "feat: ...", "--body", "..."],
  "setup_only": false
}
```

#### `write_file` — ディスクへの書き込み

```json
{
  "type": "write_file",
  "phase": "phase-5",
  "path": ".specs/20260401-fix-auth/tasks.md",
  "content": "# Tasks\n\n..."
}
```

#### `human_gate` — 外部の人間アクションを待機

```json
{
  "type": "human_gate",
  "phase": "phase-5",
  "name": "merge-external-pr",
  "present_to_user": "タスク3はrepo-bのPR #456のマージが必要です...",
  "options": ["done", "skip", "abandon"]
}
```

#### `done` — パイプライン完了

```json
{
  "type": "done",
  "summary": "Pipeline completed: 10 phases, 2 skipped",
  "summary_path": ".specs/20260401-fix-auth/summary.md"
}
```

### 4層プロンプト組み立て

`spawn_agent` アクションの `prompt` フィールドは4つの層から組み立てられる:

```
┌─────────────────────────────────────────────────────┐
│ 層1: エージェント指示                                 │
│   （agents/{name}.md からロード）                     │
├─────────────────────────────────────────────────────┤
│ 層2: 入出力アーティファクト                           │
│   ## Input Files                                    │
│   - {workspace}/request.md                          │
│   ## Output File                                    │
│   - {workspace}/design.md                           │
├─────────────────────────────────────────────────────┤
│ 層3: リポジトリプロファイル                           │
│   ## Repository Context                             │
│   Languages: Go (82%), TypeScript (15%)             │
│   Build command: make build                         │
│   Test command: go test ./...                       │
├─────────────────────────────────────────────────────┤
│ 層4: データフライホイール（パイプライン横断学習）      │
│   ## Similar Past Pipelines                         │
│   （.specs/index.jsonからBM25スコアリング）           │
│   ## Past Review Patterns                           │
│   （Levenshtein統合されたレビュー指摘）              │
│   ## AI Friction Points                             │
│   （過去のimprovement.mdレポートから）               │
└─────────────────────────────────────────────────────┘
```

層3と4はデータが利用可能な場合のみ注入される。層2はファイル**パス**のみをリスト — エージェント自身がファイルを読む。

---

## 4. `pipeline_report_result` — フェーズ結果記録

メトリクス記録、アーティファクト検証、レビュー判定解析、パイプライン状態の前進を行う。

### リクエスト

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|------|
| `workspace` | string | はい | ワークスペースパス |
| `phase` | string | はい | フェーズID（例: `"phase-3"`, `"phase-5"`） |
| `tokens_used` | number | いいえ | フェーズで消費したトークン数 |
| `duration_ms` | number | いいえ | 実時間の所要時間（ms） |
| `model` | string | いいえ | 使用モデル（例: `"sonnet"`, `"opus"`） |
| `setup_only` | boolean | いいえ | execがsetup-onlyの場合true |

### レスポンス

```json
{
  "state_updated": true,
  "artifact_written": "review-design.md",
  "verdict_parsed": "APPROVE_WITH_NOTES",
  "findings": [
    { "severity": "MINOR", "description": "Consider adding error context to..." }
  ],
  "next_action_hint": "proceed",
  "warning": "",
  "display_message": ""
}
```

### `next_action_hint` 値

| 値 | 意味 | オーケストレーターのアクション |
|-------|---------|-------------------|
| `proceed` | フェーズ正常完了 | 次の `pipeline_next_action` へ |
| `revision_required` | レビュー判定がREVISEまたはFAIL | ユーザーにフィンディングを提示、フェーズ再実行 |
| `setup_continue` | 内部セットアップアクション完了 | エンジンが自動的に `NextAction` に再入 |

### 判定解析

MCPサーバーはアーティファクト内容からレビュー判定を解析:

| フェーズ | 判定 | ソース |
|-------|----------|--------|
| phase-3b（設計レビュー） | `APPROVE`, `APPROVE_WITH_NOTES`, `REVISE` | `review-design.md` |
| phase-4b（タスクレビュー） | `APPROVE`, `APPROVE_WITH_NOTES`, `REVISE` | `review-tasks.md` |
| phase-6（コードレビュー） | `PASS`, `PASS_WITH_NOTES`, `FAIL` | `review-{N}.md` |

---

## オーケストレーターループ — 完全なデータフロー

単一フェーズに対するMCPツールコールとペイロードの正確なシーケンス:

```
オーケストレーター                     MCPサーバー                      ディスク
    │                                     │                           │
    │─── pipeline_next_action ───────────►│                           │
    │    { workspace, previous_* }        │── state.json読込 ─────────►│
    │                                     │◄── 状態データ ─────────────│
    │                                     │── Engine.NextAction() ────│
    │                                     │── agent .md読込 ──────────►│
    │                                     │── 4層プロンプト構築 ───────│
    │◄── { type: spawn_agent, ... } ──────│                           │
    │                                     │                           │
    │─── Agent(prompt=...) ──────────────────────────────────────────►│
    │                                     │       (エージェント読込)   │
    │◄── エージェント出力 ───────────────────────────────────────────│
    │                                     │                           │
    │─── Write(analysis.md) ────────────────────────────────────────►│
    │                                     │                           │
    │─── pipeline_next_action ───────────►│                           │
    │    { previous_action_complete: true  │── handleReportResult ────│
    │      previous_tokens: 15000         │── アーティファクト検証 ────►│
    │      previous_duration_ms: 45000 }  │── 判定解析 ────────────────│
    │                                     │── 状態前進 ───────────────►│
    │◄── { type: spawn_agent, ... } ──────│   (次フェーズアクション)   │
    │         (次フェーズ)                │                           │
```

> **重要な不変条件**: オーケストレーターはどのフェーズを実行するか決定しない。`pipeline_next_action` が返すアクションを実行し、結果を報告するのみ。すべての制御フローは `Engine.NextAction()` に存在する。
