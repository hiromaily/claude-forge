これは claude-forge のすべての強制メカニズムの権威あるリファレンスです。各エントリは**何が**強制されているか、**どのレイヤー**が強制しているか、**それが決定的かどうか**を文書化しています。

## 強制レイヤー

claude-forge は4つの強制レイヤーを使用しており、最も信頼性の高いものから低いものの順に並んでいます：

| レイヤー | メカニズム | 決定性 | 失敗モード |
|---|---|---|---|
| **Go MCP ハンドラー** | `tools/guards.go` のガード関数が `tools/handlers.go` のハンドラーから呼び出される。`error`（ブロッキング）または `string`（警告）を返す。 | 決定的 | `IsError=true` MCP レスポンス；状態は変更されない |
| **Go エンジン** | `orchestrator/engine.go` の決定ロジック。フェーズ遷移、auto-approve、リトライ制限、スキップゲートを制御する。 | 決定的 | 特定のアクションタイプを返す；オーケストレーターが従わなければならない |
| **シェルフック** | `scripts/` の Bash スクリプト。Claude Code フックシステム経由でツール呼び出し時に発火する。exit 2 = ブロック。 | 決定的（`jq` がない場合はフェイルオープン） | exit 2 はツール呼び出しをブロック；exit 0 は許可 |
| **プロンプト指示** | `SKILL.md` またはエージェントの `.md` ファイル内のテキスト。LLM が非決定的に従う。 | **非決定的** | LLM がスキップまたは誤解する可能性がある |

**設計原則：** すべての重要な不変条件（データ整合性、人間の承認ゲート、安全性制約）はコード（レイヤー 1–3）によって強制されます。プロンプト指示（レイヤー 4）は、コードによる強制が実用的でないオーケストレーションプロトコルへの準拠のためにのみ使用されます。

## ブロッキングガード（状態変更を防止）

これらのガードはエラーを返し、進行を停止させます。条件が満たされるまでパイプラインは進めません。

| ID | 不変条件 | レイヤー | コードの場所 | トリガー |
|---|---|---|---|---|
| 3a | フェーズ完了前にアーティファクトファイルが存在しなければならない | MCP ハンドラー | `guards.go:Guard3aArtifactExists` | 必須アーティファクトがあるフェーズの `phase_complete` |
| 3b | タスクレビューを合格としてマークする前にレビューファイルが存在しなければならない | MCP ハンドラー | `guards.go:Guard3bReviewFileExists` | `reviewStatus=completed_pass` での `task_update` |
| 3c | phase-5 開始前にタスクが初期化されていなければならない | MCP ハンドラー | `guards.go:Guard3cTasksNonEmpty` | `phase-5` の `phase_start` |
| 3e | チェックポイント完了前に `awaiting_human` ステータスが必要 | MCP ハンドラー | `guards.go:Guard3eCheckpointAwaitingHuman` | `checkpoint-a`、`checkpoint-b` の `phase_complete` |
| 3g | タスク初期化前にチェックポイント B が完了/スキップされていなければならない | MCP ハンドラー | `guards.go:Guard3gCheckpointBDoneOrSkipped` | `task_init` |
| 3j | チェックポイント完了前に保留中のリビジョンがクリアされていなければならない | MCP ハンドラー | `guards.go:Guard3jCheckpointRevisionPending` | `checkpoint-a`、`checkpoint-b` の `phase_complete` |
| — | init は事前の入力バリデーションが必要 | MCP ハンドラー | `guards.go:GuardInitValidated` | `validated=false` での `init` |
| — | アーティファクトのコンテンツがバリデーションに合格しなければならない | MCP ハンドラー | `pipeline_report_result.go` | レビューフェーズの `pipeline_report_result` |
| R1 | phase-1/2 中のソース編集ブロック（読み取り専用） | シェルフック | `pre-tool-hook.sh` ルール 1 | ワークスペース外のファイルを対象とする `Edit`/`Write` ツール |
| R2 | 並列 phase-5 実行中の Git コミットブロック | シェルフック | `pre-tool-hook.sh` ルール 2 | 並列タスクがアクティブな間の `git commit` を含む `Bash` ツール |
| R5 | アクティブなパイプライン中の main/master への Git checkout/switch ブロック | シェルフック | `pre-tool-hook.sh` ルール 5 | `git checkout main` または `git switch master` を含む `Bash` ツール |
| — | アクティブなパイプライン中の停止シグナルブロック | シェルフック | `stop-hook.sh` | ステータスが `completed`、`abandoned`、`awaiting_human` でない場合の Claude Code 停止 |

## 非ブロッキング警告（警告するが許可する）

これらのチェックは会話に警告を挿入しますが、アクションを防止しません。

| ID | チェック | レイヤー | コードの場所 | トリガー |
|---|---|---|---|---|
| 3d | フェーズログエントリの重複 | MCP ハンドラー | `guards.go:Warn3dPhaseLogDuplicate` | `phase_log` |
| 3f | フェーズ完了時のフェーズログエントリの欠落 | MCP ハンドラー | `guards.go:Warn3fPhaseLogMissing` | ログを必要とするフェーズの `phase_complete` |
| 3h | 状態にタスク番号が見つからない | MCP ハンドラー | `guards.go:Warn3hTaskNotFound` | `task_update` |
| 3i | 完了時のフェーズステータスが `in_progress` でない | MCP ハンドラー | `guards.go:Warn3iPhaseNotInProgress` | `phase_complete` |
| — | エージェント出力が短すぎる（50文字未満） | シェルフック | `post-agent-hook.sh` | アクティブなフェーズ中の `Agent` ツールの戻り値 |
| — | レビューエージェント出力に判定キーワードが欠落 | シェルフック | `post-agent-hook.sh` | `phase-3b`、`phase-4b` 中の `Agent` ツールの戻り値 |
| — | 実装レビュー出力に PASS/FAIL キーワードが欠落 | シェルフック | `post-agent-hook.sh` | `phase-6` 中の `Agent` ツールの戻り値 |

## エンジン決定（決定的な分岐）

オーケストレーターエンジン（`orchestrator/engine.go`）は状態に基づいてすべてのフェーズ遷移の決定を決定的に行います。LLM オーケストレーターは実行するアクションを受け取り、次に何をするかを自ら選択しません。

| ID | 決定 | 条件 | 動作 | コードの場所 |
|---|---|---|---|---|
| D14 | フェーズスキップ | フェーズが `skippedPhases` にある | `skip:` プレフィックスの `done` アクションを返す；オーケストレーターが `phase_complete` を呼び出す | `engine.go` `NextAction` の先頭 |
| D20 | Auto-approve（チェックポイントバイパス） | `autoApprove == true` かつ判定が `APPROVE` または `APPROVE_WITH_NOTES` | チェックポイントをバイパスして次のフェーズエージェントを起動 | `engine.go` phase-3b/4b ハンドラー |
| D21 | リトライ制限（2×） | `designRevisions >= 2` または `taskRevisions >= 2` または `implRetries >= 2` | 人間チェックポイント（承認または破棄）を強制 | `engine.go` phase-3b/4b/6 ハンドラー |
| D22 | 並列タスクディスパッチ | 最初の保留タスクが `executionMode == "parallel"` | 連続するすべての並列タスクを同時に起動 | `engine.go` phase-5 ハンドラー |
| D23 | 実装レビュー判定ルーティング | `review-N.md` から解析された判定 | FAIL → implementer を再起動；PASS → 次のタスク | `engine.go` phase-6 ハンドラー |
| D24 | PR スキップ | `skipPr == true` | `done` アクションを返し、PR 作成をバイパス | `engine.go` pr-creation ハンドラー |
| D26 | ソースへの投稿ディスパッチ | `request.md` フロントマターの `source_type` | GitHub → `gh` コマンド；Jira → チェックポイント；テキスト → done | `engine.go` post-to-source ハンドラー |

## アーティファクトバリデーション（決定的なコンテンツチェック）

`validation/artifact.go` パッケージは `pipeline_report_result` 呼び出し時にアーティファクトのコンテンツを検証します。バリデーション失敗はフェーズの進行をブロックします。

| フェーズ | 必須アーティファクト | コンテンツルール |
|---|---|---|
| phase-1 | `analysis.md` | `## ` 見出しを含まなければならない |
| phase-2 | `investigation.md` | `## ` 見出しを含まなければならない |
| phase-3 | `design.md` | `## ` 見出しを含まなければならない |
| phase-3b | `review-design.md` | `APPROVE`、`APPROVE_WITH_NOTES`、または `REVISE` を含まなければならない |
| phase-4 | `tasks.md` | `## Task` 見出しを含まなければならない |
| phase-4b | `review-tasks.md` | `APPROVE`、`APPROVE_WITH_NOTES`、または `REVISE` を含まなければならない |
| phase-6 | `review-N.md` | `PASS`、`PASS_WITH_NOTES`、または `FAIL` を含まなければならない |
| phase-7 | `comprehensive-review.md` | 空でなければならない |
| final-summary | `summary.md` | 存在しなければならない |

所見マーカー（`[CRITICAL]`、`[MINOR]`）はカウントされ、履歴分析のためのパターン知識ベースに蓄積されます。これは非ブロッキングです。

## 自動化された副作用（決定的なアクション）

| アクション | レイヤー | コードの場所 | トリガー |
|---|---|---|---|
| 最終コミット：`summary.md` + `state.json` を最後のコミットに amend してから force-push | シェルフック（v1）/ エンジン exec アクション（v2） | `post-bash-hook.sh`（v1 レガシー）/ `engine.go` final-commit アクション（v2） | `post-to-source` フェーズ完了後；state.json がコミット時に "completed" 状態になるよう `pipeline_report_result` を先に呼び出す |
| リビジョンカウンターのインクリメント | MCP ハンドラー | `pipeline_report_result.go` | レビューフェーズの `REVISE` 判定 |
| パターン知識の蓄積 | MCP ハンドラー | `pipeline_report_result.go` | 所見を伴う任意のレビューフェーズ完了 |

## プロンプトのみの指示（非決定的）

これらの動作は **LLM 指示のみによって**強制されます。オーケストレーションレベルの決定を含み、状態ガードとして表現することが実用的でないため、コードによる保証はできません。

| 指示 | 場所 | コードで強制しない理由 |
|---|---|---|
| Agent 呼び出しに `isolation: "worktree"` を渡さない | SKILL.md | Claude Code の Agent ツールパラメータ；Agent ツール引数のフックインターセプトポイントがない |
| `spawn_agent`、`exec`、`write_file` の後に常に `pipeline_report_result` を呼び出す | SKILL.md | 省略は no-op（メトリクス欠落）であり、状態の破損ではない；タイムアウトガードを追加するとリスクに対して不釣り合いな複雑さが加わる |
| チェックポイントで `phase_complete` を呼び出す前に人間の応答を待つ | SKILL.md | **待機**自体はプロンプトのみだが、**ゲート**は決定的：ガード 3e は `checkpoint()` が最初に呼び出されない限り `phase_complete` をブロックするため、LLM はチェックポイントをスキップできない |
| `done` アクションの `skip:` プレフィックスを解析して `phase_complete` を呼び出す | SKILL.md | エンジンがスキップシグナルを返す；オーケストレーターが解析に失敗した場合、`pipeline_next_action` は同じスキップシグナルを再び返す（自己修正ループ） |

## デュアルレイヤー強制マップ

一部の不変条件は、シェルフックレイヤーと Go MCP ハンドラーレイヤーの両方で強制されます。これにより多層防御が提供されます：MCP ハンドラーは MCP ツール呼び出し時に最初に発火し、シェルフックは `Bash`/`Edit`/`Write` ツール呼び出し時に独立して発火します。

| 不変条件 | シェルフック | MCP ハンドラー |
|---|---|---|
| フェーズ進行前にアーティファクトが存在しなければならない | — | ガード 3a（`phase_complete`）+ `pipeline_report_result` バリデーション |
| チェックポイントには人間の承認が必要 | — | ガード 3e（`phase_complete` には `awaiting_human` が必要） |
| Phase 1-2 読み取り専用 | ルール 1（`pre-tool-hook.sh`） | —（エージェントはファイル編集に MCP ツールを呼び出さない） |
| 並列 git コミットなし | ルール 2（`pre-tool-hook.sh`） | —（git コミットは MCP ではなく `Bash` ツールを経由） |
| main/master への checkout なし | ルール 5（`pre-tool-hook.sh`） | —（ブランチ操作は `Bash` ツールを経由） |
| レビュー判定の抽出 | シェル警告（`post-agent-hook.sh`） | アーティファクトコンテンツバリデーション（`validation/artifact.go`） |

## フェイルオープン保証

すべてのシェルフックはフェイルオープンです：`jq` がインストールされていないか `state.json` が読み取れない場合、フックは exit 0（許可）します。これにより、プラグインが正当な非パイプライン作業をブロックしないことが保証されます。

```bash
# すべてのフックはこのパターンで始まる:
command -v jq >/dev/null 2>&1 || exit 0
```
