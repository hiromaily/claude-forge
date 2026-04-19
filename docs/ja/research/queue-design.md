# forge-queue: 自律タスクキュー設計

ステータス: draft v3 (2026-04-17)

## 概要

`forge-queue` はイシューベースのタスクを逐次バッチ実行する機能を提供します。
ユーザーはイシュー URL のリストを含む `.specs/queue.yaml` を作成し、MCP サーバーがキューの状態を管理しながら、既存の `forge` パイプラインが各タスクを処理します。

## アーキテクチャ

```text
SKILL.md (forge-queue)
  │
  │  queue_init(queue_path)          ← parse + validate YAML
  │        │
  │        ▼
  │  queue_next(queue_path)          ← return next task + pre-generated workspace slug
  │        │
  │        ▼
  │  claude -p "/forge {url} --auto" ← isolated subprocess, forge unchanged
  │        │                            SKILL.md passes workspace_slug in user_confirmation
  │        ▼
  │  queue_report(queue_path, index) ← find workspace by slug, read state.json
  │        │
  │        ▼
  │  queue_next(queue_path)          ← next task, or "all done"
  │        │
  │        ...
```

4 つの新しい MCP ツールはシンプルな YAML I/O ラッパーです。すべてのパイプラインロジックは既存の `pipeline_init` / `pipeline_next_action` / `pipeline_report_result` チェーンに残ります。

## スキル

責務が明確に分離された 2 つのスキル:

- `/forge-queue-create` — `.specs/queue.yaml` を生成する
- `/forge-queue` — `.specs/queue.yaml` を実行する

### `/forge-queue-create`

`queue.yaml` ファイルを生成します。2 つの入力モードをサポートします:

**モード A: URL 直接指定**

```text
/forge-queue-create https://jira.example.com/browse/DEA-123 https://jira.example.com/browse/DEA-456
```

スキルは `queue_create` MCP ツールで各 URL をバリデートし、ユーザーにエフォートレベルを確認（またはデフォルトを受け入れ）して YAML ファイルを書き込みます。

**モード B: 検索ベースの収集**

```text
/forge-queue-create --jira-project DEA --jira-status "To Do"
/forge-queue-create --gh-label "bug" --gh-state "open"
```

スキルは既存ツールを使ってイシューを検索します:

- **Jira**: Atlassian MCP ツール（利用可能な場合）または Jira REST API（forge の Jira 連携と同パターンで `curl` を使用）
- **GitHub**: `gh issue list --label <label> --state <state> --json url,title`

スキルは一致するイシューを収集してユーザーに確認（選択/解除）を求め、タスクごとのエフォートを確認（またはデフォルト）してから `queue_create` を呼び出して YAML ファイルを書き込みます。

**この分割の理由**: モード A は決定論的（URL → YAML）であり、MCP ツールで完全に処理できます。モード B は外部 API 呼び出しとユーザー操作（イシュー選択）が必要であり、これはスキルレベルの関心事です — MCP ツールは Jira/GitHub への API 呼び出しやユーザーとのやり取りを行うべきではありません。

### `/forge-queue`

キューを実行します。以下の[スキル設計](#スキル設計)を参照してください。

## MCP ツール

### `queue_create`

**目的**: URL リストから新しい `.specs/queue.yaml` を生成する。

**パラメーター**:

- `queue_path` (string, 必須): キュー YAML ファイルの出力パス。
- `tasks` (array, 必須): タスクオブジェクトのリスト。各オブジェクトは以下を含む:
  - `url` (string, 必須): イシュー URL。
  - `effort` (string, 任意): `S`、`M`、または `L`。省略した場合、forge は推奨エフォートを自動選択する（`--auto` の動作）。

**動作**:

1. 各エントリをバリデート:
   - `url` が既知のソースタイプ（GitHub イシュー、Jira イシュー）と一致するか確認。
   - `effort` が存在する場合、`S`、`M`、`L` のいずれかであることを確認。
2. ファイルが既に存在する場合、エラーを返す（誤った上書きを防止）。ユーザーは先に既存ファイルを削除またはリネームする必要があります。
3. YAML ファイルを書き込む。

**返り値**:

```json
{
  "created": true,
  "path": ".specs/queue.yaml",
  "task_count": 3,
  "errors": []
}
```

### `queue_init`

**目的**: `.specs/queue.yaml` を解析してバリデートする。

**パラメーター**:

- `queue_path` (string, 必須): キュー YAML ファイルへのパス。

**動作**:

1. YAML ファイルを読み込んで解析する。
2. 全エントリをバリデート:
   - `url` が存在し、空でなく、既知のソースタイプ（GitHub イシュー、Jira イシュー）と一致するか確認。
   - `effort` が存在する場合、`S`、`M`、`L` のいずれかであることを確認。
3. サマリーを返す: 合計数、完了数、失敗数、保留数。

**返り値**:

```json
{
  "total": 4,
  "completed": 1,
  "failed": 0,
  "pending": 3,
  "errors": []
}
```

`errors` が空でない場合、キューは無効であり処理すべきではありません。

### `queue_next`

**目的**: キューから次の未処理タスクを返し、ワークスペーススラグを事前生成してワークスペースパスを決定論的にする。

**パラメーター**:

- `queue_path` (string, 必須): キュー YAML ファイルへのパス。

**動作**:

1. YAML ファイルを読み込んで解析する。
2. `status` が存在しない**または** `status` が `in_progress`（割り込み後の再開）である最初のエントリを見つける。
3. 新規タスクの場合:
   - URL からワークスペーススラグを事前生成する。スラグはイシュー識別子から導出される:
     - Jira: `dea-123`（`https://jira.example.com/browse/DEA-123` から）
     - GitHub: `42`（`https://github.com/org/repo/issues/42` から）
     スラグは URL のみから導出された安定した決定論的な値です。
   - queue.yaml に `status: in_progress`、`started_at: <ISO8601>`、`workspace_slug: <生成スラグ>` を書き込む。
   `in_progress` エントリの場合: 変更なし（冪等 — `started_at` と `workspace_slug` は前回の試行から保持される）。
4. `workspace_slug` を含むタスク詳細を返す。

**ワークスペーススラグが forge に届く仕組み**: forge サブプロセスの SKILL.md は `pipeline_init_with_context` への `user_confirmation` オブジェクトで `workspace_slug` を渡します。これは**既存の機能**です — `pipeline_init_with_context` はすでに `user_confirmation` で `workspace_slug` を受け入れ、`applyWorkspaceSlug`（`pipeline_init_with_context.go` の 277-283 行目）で適用します。forge のコード変更は不要です。

実際のワークスペースパスは forge が決定します:
`YYYYMMDD-{source_id}-{workspace_slug}` または
`YYYYMMDD-{source_id}-{issue_title_slug}`（外部コンテキストでスラグが精緻化された場合）。`queue_report` は日付 + source_id プレフィックスで `.specs/` をスキャンしてワークスペースを特定します。

**再開セマンティクス**: `in_progress` エントリは前のセッションがパイプライン途中で割り込まれたことを意味します。`queue_next` はそれを次のタスクとして返し、forge パイプラインの既存の再開ロジック（`pipeline_init` の自動再開）が回復を処理します。特別なキューレベルの再試行ロジックは不要です。

**返り値**（エフォートありの新規タスク）:

```json
{
  "has_next": true,
  "index": 2,
  "resuming": false,
  "url": "https://github.com/org/repo/issues/42",
  "effort": "S",
  "workspace_slug": "42",
  "forge_arguments": "https://github.com/org/repo/issues/42 --auto effort:S"
}
```

**返り値**（エフォートなしの新規タスク）:

```json
{
  "has_next": true,
  "index": 3,
  "resuming": false,
  "url": "https://jira.example.com/browse/DEA-789",
  "effort": null,
  "workspace_slug": "dea-789",
  "forge_arguments": "https://jira.example.com/browse/DEA-789 --auto"
}
```

`forge_arguments` は `pipeline_init(arguments=...)` に直接渡せる事前構築済み文字列です。`--auto` フラグは常に含まれます。`effort` がない場合、`effort:` フラグは省略されます — forge の `pipeline_init_with_context` は `--auto` モードで推奨エフォートを自動選択します。

`forge_arguments` にワークスペーススラグは含まれません。スラグは別途渡されます — forge-queue の SKILL.md はそれを `claude -p` プロンプトに埋め込み、サブプロセスの forge SKILL.md が `user_confirmation` に含めます。

**返り値**（割り込まれたタスクの再開）:

```json
{
  "has_next": true,
  "index": 1,
  "resuming": true,
  "url": "https://jira.example.com/browse/DEA-456",
  "effort": "S",
  "workspace_slug": "dea-456",
  "workspace": ".specs/20260417-dea-456-add-export-feature",
  "forge_arguments": ".specs/20260417-dea-456-add-export-feature"
}
```

`resuming` が true の場合、`forge_arguments` には URL の代わりにワークスペースパスが含まれます。forge の `pipeline_init` はこれを再開候補として検出し、自動再開で処理します。`workspace` フィールドは queue.yaml から読み込まれます（最初の試行後に `queue_report` が書き込む）。

**返り値**（タスクなし）:

```json
{
  "has_next": false,
  "summary": {
    "total": 4,
    "completed": 3,
    "failed": 1,
    "results": [
      {"url": "...", "status": "completed", "pr": 2891},
      {"url": "...", "status": "failed", "reason": "..."}
    ]
  }
}
```

### `queue_report`

**目的**: 完了したタスクの結果を決定し、`queue.yaml` に記録する。呼び出し元がパイプラインの結果を解釈する必要はなく、このツールが直接 `state.json` を読み込んで結果を決定する（決定論的、LLM の判断不要）。

**パラメーター**:

- `queue_path` (string, 必須): キュー YAML ファイルへのパス。
- `index` (number, 必須): `queue_next` が返したタスクインデックス。

**動作**:

1. `queue.yaml` を読み込み、`index` のエントリを見つける。
2. エントリから `workspace_slug` を読み込む（`queue_next` が書き込んだもの）。
3. `.specs/` でワークスペースディレクトリを特定する:
   - `started_at` から日付プレフィックスを抽出する（例: `20260417`）。
   - URL から source ID を抽出する（例: Jira は `dea-123`、GitHub は `42`）。
   - `.specs/` でパターン `{date_prefix}-{source_id}*` に一致するディレクトリをスキャンする。実行が逐次でありソース ID がキューエントリごとにユニークなため、最大 1 つのディレクトリが一致します。
   - 一致が見つからない場合: タスクを `failed`（理由: `"workspace not found"`）としてマークする。
4. `{workspace}/state.json` を読み込む。
5. 結果を決定論的に決定する:
   - `currentPhase == "completed"` → `status: completed`。
   - その他のフェーズ → `status: failed`。
     理由: `state.json` から `"{currentPhase}: {error.message}"`。
     `state.Error` が nil の場合（エラーなしで放棄されたパイプラインなど）、理由は `"{currentPhase}: abandoned"`。
6. ブランチ名のために `state.json.branch` を読み込む。
7. queue.yaml エントリを更新する:
   - `status`: completed または failed
   - `workspace`: 実際のディレクトリ名（例: `20260417-dea-123-fix-login`）
   - `branch`: git ブランチ名（例: `feature/20260417-dea-123-fix-login`）
   - `reason`: 失敗理由（failed のみ）
   - `finished_at`: ISO8601 タイムスタンプ
8. `queue.yaml` をアトミックに書き込む。

**返り値**:

```json
{
  "status": "completed",
  "branch": "feature/20260417-dea-123-fix-login-validation",
  "workspace": "20260417-dea-123-fix-login-validation",
  "remaining": 2
}
```

### `queue_update_pr`

**目的**: queue.yaml エントリに PR 番号を書き込む。`gh pr list` で PR を検索した後にスキルが呼び出す。

**パラメーター**:

- `queue_path` (string, 必須): キュー YAML ファイルへのパス。
- `index` (number, 必須): タスクインデックス。
- `pr` (number, 必須): PR 番号。

**動作**:

1. `queue.yaml` を読み込み、`index` のエントリを見つける。
2. エントリに `pr: <number>` を書き込む。
3. `queue.yaml` をアトミックに書き込む。

**返り値**:

```json
{
  "updated": true
}
```

**設計の根拠**: PR 番号の検索には `gh pr list`（シェルコマンド）が必要ですが、これは MCP ツール内で実行してはなりません（制約 #12）。スキルがシェルコマンドを実行し、結果をこのツールに渡してアトミックな YAML 書き込みを行います。これにより MCP ツールをピュアな Go で保ちつつ、queue.yaml の書き込みが常にアトミックであることを保証します（制約 #6）。

## スキル設計

`forge-queue` は forge の内部を何も知らない独立したスキル（`/forge-queue`）です。各タスクは**隔離された `claude -p` サブプロセス**で実行され、タスクごとにクリーンなコンテキストウィンドウを確保してタスク間の汚染ゼロを実現します。

```markdown
## Step 1: Initialize

1. Call `queue_init(queue_path=".specs/queue.yaml")`.
2. If errors: report and stop.
3. Report queue status (e.g. "4 tasks: 1 completed, 1 failed, 2 pending").

## Step 2: Process Loop

1. Call `queue_next(queue_path=".specs/queue.yaml")`.
2. If `has_next` is false: output summary and stop.
3. If NOT resuming (`resuming` is false):
   Run `git checkout main && git pull --rebase`.
4. Run forge as a subprocess via Bash:
   `claude -p "/forge {forge_arguments}" --allowedTools "Bash,Read,Write,Edit,Glob,Grep,Agent,Skill,mcp__plugin_claude-forge_forge-state__*"`
   - For new tasks, append to the prompt:
     "Use workspace_slug '{workspace_slug}' in user_confirmation."
   - Each subprocess starts a fresh session with an empty context window.
   - forge runs autonomously (--auto) and exits on completion or failure.
5. Call `queue_report(queue_path=".specs/queue.yaml", index=<index>)`.
6. If `status == "completed"` and `branch` is present:
   a. Run `gh pr list --head {branch} --json number --jq '.[0].number'`
   b. If PR number is found:
      Call `queue_update_pr(queue_path, index, pr=<number>)`.
7. Return to step 1.
```

### サブプロセス隔離の理由

- **コンテキスト分離**: 各タスクはクリーンなコンテキストウィンドウを得る。前のタスクのコード、エラー、設計上の決定が次のタスクに漏れない。
- **/clear 不要**: `/clear` は CLI のみのインタラクティブコマンドであり、プログラム的に呼び出すことができない。`claude -p` はタスクごとに新しいセッションを開始することで同じ効果を実現する。
- **forge 変更なし**: forge の観点から見ると、各サブプロセスの呼び出しは、ユーザーが新しいターミナルで `/forge {url} --auto` と入力するのと同一です。

### サブプロセスでの MCP サーバーの利用可能性（確認済み）

`claude -p` サブプロセスは、`claude-forge` がプラグインとしてインストールされているリポジトリのルートから実行された場合、`forge-state` MCP サーバーへの完全なアクセスを持ちます。**確認済み**: すべての 46 個の `mcp__plugin_claude-forge_forge-state__*` ツールが `claude -p` セッションで利用可能です（2026-04-17 にテスト済み）。

認証（`gh` CLI、Jira 認証情報）は親シェル環境から継承されます。

### ワークスペーススラグのフロー

ワークスペーススラグは forge を変更することなくシステムを通じて流れます:

```text
queue_next                    queue.yaml          subprocess (forge)
  │                               │                     │
  │ pre-generate slug             │                     │
  │ from URL (e.g. "dea-123")    │                     │
  │──write workspace_slug───────▶│                     │
  │                               │                     │
  │ return workspace_slug         │                     │
  │◀──────────────────────────────│                     │
  │                               │                     │
  │ embed slug in claude -p       │                     │
  │ prompt instruction            │                     │
  │──────────────────────────────────────────────────▶ │
  │                               │    forge SKILL.md   │
  │                               │    passes slug in   │
  │                               │    user_confirmation│
  │                               │    .workspace_slug  │
  │                               │         │           │
  │                               │         ▼           │
  │                               │    pipeline_init_   │
  │                               │    with_context     │
  │                               │    applies slug     │
  │                               │    (existing code   │
  │                               │     L277-283)       │
  │                               │         │           │
  │                               │         ▼           │
  │                               │    workspace created│
  │                               │    .specs/20260417- │
  │                               │    dea-123-fix-login│
  │                               │                     │

queue_report                  queue.yaml
  │                               │
  │ read workspace_slug           │
  │◀──────────────────────────────│
  │                               │
  │ scan .specs/ for              │
  │ {date}-{source_id}*           │
  │ → finds 20260417-dea-123-...  │
  │                               │
  │ read state.json               │
  │ determine status              │
  │──write workspace, branch─────▶│
```

### 再開の動作

ユーザーがキューの実行を割り込んだ場合（Ctrl+C、ターミナルを閉じるなど）:

1. 現在のタスクの `status` は `queue.yaml` で `in_progress` のまま残り、`workspace_slug` と `started_at` はすでに記録されています。
2. 完了したタスクはすでに `completed` または `failed` です。
3. 残りのタスクは `status` を持ちません。

再開するには、ユーザーが再び `/forge-queue .specs/queue.yaml` を実行するだけです:

1. `queue_init` が現在の状態を報告する（N completed、M failed、1 in progress、K pending）。
2. `queue_next` が既存の `workspace_slug` を持つ `in_progress` タスクを返す。
3. `workspace` が設定されている場合（最初の部分実行後に `queue_report` が書き込んだ場合）:
   `forge_arguments` にはワークスペースパスが含まれ、`pipeline_init` を通じて forge の自動再開をトリガーします。
4. `workspace` がまだ設定されていない場合（`queue_report` が実行される前に割り込まれた場合）:
   `forge_arguments` には URL が含まれます。forge は新しいワークスペースを作成します。ワークスペーススラグにより同じスラグが使用されますが、`pipeline_init` は若干異なるワークスペース名を生成する可能性があります（日付が異なる場合があります）。`queue_report` は日付プレフィックスのスキャンによってこれを処理します。
5. 再開したタスクが完了した後、`queue_report` が結果を記録し、ループは次の保留タスクで続きます。

### 関心の分離

`forge-queue` が**知らない**こと:

- forge のメインループの仕組み（`pipeline_next_action` のディスパッチ）
- アクションタイプの種類（`spawn_agent`、`checkpoint`、`exec` など）
- フェーズ、リビジョン、レビューの仕組み
- PR 作成やソースへの投稿の仕組み

`forge-queue` が**知っていること**:

- YAML キューの読み込み/バリデーション方法（`queue_init`）
- 次のタスクを選択してスラグを生成する方法（`queue_next`）
- `forge_arguments` を使って `claude -p` サブプロセスを起動する方法
- 特定の `workspace_slug` を使用するようサブプロセスに指示する方法
- 結果を記録する方法（`queue_report`）
- `gh pr list` を使って PR 番号を検索する方法（シェルコマンド）
- PR 番号をアトミックに書き戻す方法（`queue_update_pr`）
- タスク間でメインブランチに戻る方法

## queue.yaml スキーマ

```yaml
tasks:
  - url: https://jira.example.com/browse/DEA-123    # required
    effort: M                                        # optional: S | M | L (auto-selected if omitted)
    # — fields below are managed by forge-queue —
    status: completed                                # completed | failed | in_progress
    workspace_slug: dea-123                          # pre-generated slug (set by queue_next)
    workspace: 20260417-dea-123-fix-login             # actual .specs/ directory name (set by queue_report)
    branch: feature/20260417-dea-123-fix-login        # git branch name (set by queue_report)
    pr: 2891                                         # PR number (set by skill via queue_update_pr)
    reason: "phase-3: design rejected"               # failure reason (set by queue_report)
    started_at: "2026-04-17T10:30:00Z"               # ISO8601 (set by queue_next)
    finished_at: "2026-04-17T10:45:00Z"              # ISO8601 (set by queue_report)
```

## 設計上の制約

1. **逐次のみ** — 並列実行なし。ユーザーは並列処理のために複数のターミナルを開く。
2. **`--auto` 強制** — チェックポイントなし。各タスクは自律的に実行される。
3. **リンクのみの入力** — タスクはイシュー URL（Jira、GitHub）でなければならない。フリーテキストのタスクはキューモードではサポートされない。
4. **forge の内部変更なし** — 5 つのキューツールは追加的なものです。`pipeline_init`、`pipeline_next_action`、`pipeline_report_result` は変更されない。ワークスペーススラグは既存の `user_confirmation.workspace_slug` フィールドを通じて伝達される（すでに `pipeline_init_with_context` でサポート済み）。
5. **キューの状態は queue.yaml に保存** — YAML ファイルは入力と状態トラッカーの両方を兼ねる。別の状態ファイルはない。
6. **アトミックな書き込み** — すべての queue.yaml の変更は MCP ツール（`queue_next`、`queue_report`、`queue_update_pr`）を通じて行われる。スキルは直接 queue.yaml を書き込まない。
7. **ブランチ隔離** — 各タスクは独自のブランチを持つ。スキルはタスク間で `git checkout main && git pull --rebase` を実行する。
8. **フェイルフォワード** — 失敗したタスクは放棄され、次のタスクが開始される。
9. **決定論的な結果決定** — `queue_report` は直接 state.json を読み込む。SKILL.md はパイプラインの結果を解釈しない。
10. **再開可能** — `queue_next` は `in_progress` エントリを候補として扱い、割り込まれたセッションからの回復を可能にする。
11. **セッション隔離** — 各タスクは別の `claude -p` サブプロセスで実行される。コンテキストウィンドウはタスクごとにクリーン。タスク間の汚染なし。
12. **MCP ツールはピュアな Go** — MCP ツール内で外部コマンド（`gh`、`curl` など）の `os/exec` 呼び出しなし。シェルコマンドはスキルレイヤーのみで実行される。
13. **ワークスペーススラグはサブプロセスより先に判明** — `queue_next` がスラグを事前生成して queue.yaml に書き込む。サブプロセスは既存の `user_confirmation.workspace_slug` メカニズムを通じてそれを forge に渡す。`queue_report` は日付 + source_id プレフィックスのスキャンでワークスペースを特定する。

## Go パッケージの配置

```text
mcp-server/internal/queue/         ← YAML parse/validate/read/write + workspace scan
mcp-server/internal/handler/tools/
  queue_create.go                  ← MCP handler (generate queue.yaml)
  queue_init.go                    ← MCP handler (validate existing queue.yaml)
  queue_next.go                    ← MCP handler (pick next task + slug)
  queue_report.go                  ← MCP handler (record result)
  queue_update_pr.go               ← MCP handler (write PR number)
skills/
  forge-queue/SKILL.md             ← queue executor skill
  forge-queue-create/SKILL.md      ← queue generator skill
```

### 依存関係の方向

`queue` パッケージは `state.ReadState`（読み取り専用）をインポートして `queue_report` でパイプラインの結果を決定します。これは一方向の依存関係です:

```text
tools → queue → state (ReadState only)
```

これは既存のレイヤリングルール（`tools → ... → state`）に従います。逆方向の依存関係は導入されません。`queue` パッケージは `engine/orchestrator` または `handler/tools` をインポートしません。

### URL バリデーションの再利用

`queue_create` と `queue_init` の両方がソースタイプ検出を使用して URL をバリデートします。`handler/validation` パッケージ（`pipeline_init` が使用）はソースタイプ検出を含む `ValidateInput` を公開しています。`queue` は `engine/state` のみをインポートするため（`handler/tools` や `handler/validation` はインポートしない）、URL バリデーションロジックは `handler/validation` パッケージ内の共有関数として抽出されます（`queue` は DAG に違反することなくこれをインポートできます）:

```text
tools → queue → validation (URL validation)
                      ↑
              tools → validation (existing)
```

## テスト戦略

### Go ユニットテスト（`mcp-server/internal/queue/`）

- YAML 解析/書き込みのラウンドトリップ（フィールド順を保持）
- バリデーション: URL 欠如、無効なエフォート、無効な URL フォーマット、重複 URL
- `queue_next` の状態遷移: 未設定 → in_progress、in_progress → 冪等
- `queue_next` で全タスク完了 → `has_next: false`
- `queue_next` スラグ生成: Jira URL → 小文字キー、GitHub URL → イシュー番号
- ワークスペーススキャン: 候補が 0、1、複数ある場合の日付 + source_id プレフィックスマッチング
- state.json からの `queue_report` ステータス決定:
  - `currentPhase == "completed"` → completed
  - `currentPhase != "completed"`、`Error` あり → failed with message
  - `currentPhase != "completed"`、`Error` なし → failed with "abandoned"
- `queue_report` ワークスペーススラグの精緻化: 事前生成スラグ vs 実際のディレクトリ
- アトミック書き込み: 書き込み後のファイルの整合性を確認

### MCP ハンドラーテスト（`mcp-server/internal/handler/tools/`）

- `queue_create`: URL をバリデート、既存ファイルを拒否、有効な YAML を書き込む
- `queue_init`: 混合ステータスキューの正しいカウントを返す
- `queue_next`: エフォートあり/なしで正しい `forge_arguments` を返す
- `queue_next`: Jira と GitHub URL で正しい `workspace_slug` を返す
- `queue_report`: state.json を読み込み、正しいステータスとブランチを書き込む
- `queue_update_pr`: 正しいエントリに PR 番号を書き込む

### 統合テスト（手動）

- エンドツーエンド: キュー作成 → `/forge-queue` 実行 → queue.yaml が更新されたことを確認
- タスク途中で割り込み → 再開 → `in_progress` タスクが取得されることを確認
- `user_confirmation` の `workspace_slug` が期待されるワークスペースパスを生成することを確認

## ツール数への影響

現在: 46 ツール。変更後: 51 ツール（+5: `queue_create`、`queue_init`、`queue_next`、`queue_report`、`queue_update_pr`）。
`CLAUDE.md`、`scripts/README.md`、`README.md` のカウントを更新してください。
