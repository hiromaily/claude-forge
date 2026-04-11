claude-forge は10の専門エージェントを使用し、各エージェントがパイプラインの1つのフェーズを担当します。すべてのエージェントは `.md` ファイルで定義された独自のシステムプロンプトを持つ隔離されたコンテキストウィンドウで実行されます。

## エージェント一覧

| エージェント | フェーズ | 役割 |
|------------|---------|------|
| [situation-analyst](/ja/agents/situation-analyst) | 1 | 読み取り専用のコードベース探索 — ファイル、インターフェース、型、データフローのマッピング |
| [investigator](/ja/agents/investigator) | 2 | 深掘り調査 — 根本原因、エッジケース、統合ポイント |
| [architect](/ja/agents/architect) | 3 | ソフトウェア設計 — アプローチ、アーキテクチャ、データモデル、テスト戦略 |
| [design-reviewer](/ja/agents/design-reviewer) | 3b | 設計品質ゲート — APPROVE または REVISE |
| [task-decomposer](/ja/agents/task-decomposer) | 4 | 設計を番号付き依存関係対応タスクに分解 |
| [task-reviewer](/ja/agents/task-reviewer) | 4b | タスクリスト品質ゲート — APPROVE または REVISE |
| [implementer](/ja/agents/implementer) | 5 | TDD開発者 — テストファースト、1タスクずつ |
| [impl-reviewer](/ja/agents/impl-reviewer) | 6 | diff ベースのコードレビュー — PASS、PASS_WITH_NOTES、または FAIL |
| [comprehensive-reviewer](/ja/agents/comprehensive-reviewer) | 7 | タスク横断の総合レビュー — 命名、重複、整合性 |
| [verifier](/ja/agents/verifier) | 最終 | フルタイプチェックとテストスイート実行、新規の失敗を修正 |

## 呼び出し方法

エージェントはオーケストレーターが **Agent ツール** でエージェントの `name` を指定して呼び出します。オーケストレーターはランタイムパラメータ（ワークスペースパス、タスク番号）のみを渡し、エージェントの命令は自己完結しています。

## タスクタイプ別スキップ

すべてのエージェントがすべてのタスクタイプで実行されるわけではありません：

| エージェント | feature | bugfix | refactor | docs | investigation |
|------------|:-------:|:------:|:--------:|:----:|:-------------:|
| situation-analyst | ✅ | ✅ | ✅ | ✅ | ✅ |
| investigator | ✅ | ✅ | ✅ | — | ✅ |
| architect | ✅ | ✅ | ✅ | — | ✅ |
| design-reviewer | ✅ | — | — | — | ✅ |
| task-decomposer | ✅ | — | ✅ | — | — |
| task-reviewer | ✅ | — | — | — | — |
| implementer | ✅ | ✅ | ✅ | ✅ | — |
| impl-reviewer | ✅ | ✅ | ✅ | ✅ | — |
| comprehensive-reviewer | ✅ | — | ✅ | — | — |
| verifier | ✅ | ✅ | ✅ | ✅ | — |

## モデル設定

すべてのエージェントはデフォルトでコスト最適化のため `model: sonnet` を使用。より強力な推論が必要な場合は `.md` ファイルのフロントマターで個別に `opus` にアップグレードできます。
