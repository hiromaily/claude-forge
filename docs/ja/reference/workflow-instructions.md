# ワークフロールール (`.specs/instructions.md`)

特定の条件にマッチするタスクに対して `mode: human_gate` を決定論的に強制できる、リポジトリ単位のファイルです。ルールは phase-4 の完了時に Go のコードで評価されます。強制処理に LLM は関与しません。

## ファイルの配置場所

リポジトリ**ルート**の `.specs/instructions.md`。このファイルはオプションで、存在しなければパイプラインは従来どおり動作します。

## ファイル形式

YAML frontmatter を 1 ブロック持つ Markdown ファイルです。バリデータが読むのは frontmatter のみで、Markdown 本文は人間向けのメモとして扱われます。

```markdown
---
rules:
  - id: akupara-proto
    when:
      files_match:
        - "backend/**/*.proto"
        - "backend/gen/proto/**"
    require: human_gate
    reason: "akupara-proto の PR マージ状態を確認してください"

  - id: destructive-migration
    when:
      files_match:
        - "backend/migrations/**/*.sql"
      title_matches: "(?i)drop\\s+(table|column)"
    require: human_gate
    reason: "破壊的マイグレーションのため stakeholder 確認が必要です"
---

# Notes (ignored by validator)

Any markdown after the closing `---` is free-form documentation for humans.
```

## ルールのスキーマ

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `id` | string | yes | エラーメッセージで使用される一意の識別子 |
| `when.files_match` | list of glob | files_match / title_matches のいずれか必須 | 各タスクの `files:` リストに対して評価される doublestar の glob パターン |
| `when.title_matches` | Go regex | 上に同じ | タスクタイトルに対して評価される正規表現 |
| `require` | string | yes | MVP では `human_gate` のみ |
| `reason` | string | yes | 該当タスクに到達した際の human_gate プロンプトで表示される |

### `when` のセマンティクス

- `files_match` は複数パターン間で OR（いずれかの glob がいずれかのファイルにマッチすればヒット）。
- `title_matches` と `files_match` が両方指定されている場合は AND で結合。
- Glob は [`doublestar` 構文](https://github.com/bmatcuk/doublestar)を使用。`**` は任意の数のパスセグメントにマッチします。
- 正規表現は Go 標準ライブラリの `regexp` パッケージを使用。大文字小文字を無視したい場合は `(?i)` を付けてください。

## 評価フロー

1. task-decomposer が phase-4 で実行され、`tasks.md` を書き出す。
2. `pipeline_report_result` が `phase=phase-4` で呼び出される。
3. Go のバリデータがリポジトリルートの `.specs/instructions.md` を読み込む。
4. `tasks.md` の各タスクをすべてのルールに対してチェックする。
5. タスクがあるルールの `when` にマッチしているにもかかわらず `mode: human_gate` を持っていない場合、違反として記録される。
6. 違反が 1 件でもあれば `review-tasks.md` が書き出され、`next_action_hint` が `revision_required` に設定され、既存の revision ループが task-decomposer を再実行する。phase-4 は完了**しない**。
7. 違反がゼロの場合、phase-4 は通常どおり phase-4b（AI task-reviewer）に進む。

## 失敗モード

| 条件 | 挙動 |
|---|---|
| ファイルが存在しない | ルールなし、パイプラインは従来どおり |
| frontmatter が欠落 | ハードエラー: "missing YAML frontmatter" |
| 未知のフィールド（タイポ） | ハードエラー: "field X not found" |
| `require:` が `human_gate` でない | ロード時にハードエラー |
| `title_matches` の正規表現が無効 | ロード時にハードエラー |
| `when` 条件を持たないルール | ロード時にハードエラー |
| glob がどのタスクにもマッチしない | エラーではない（単にルールが発動しないだけ） |

## 例: dealon-app

dealon-app リポジトリでは、特に以下の 2 つのルールが有用です。

1. **akupara-proto 連携** — `.proto` ファイルに触れるタスクは、外部の akupara-proto PR が先にマージされている必要がある。
2. **破壊的マイグレーションの承認** — `DROP TABLE` / `DROP COLUMN` を含む SQL マイグレーションは stakeholder の承認が必要。

どちらも `files_match` + `title_matches` で表現でき、LLM の判断を必要としない純粋なパターンマッチで済みます。

## サポート対象外（スコープ外）

- LLM エージェントが解釈する自然言語ルール。
- ソースファイルの内容を読み取ること（タスクの `files:` とタイトルのみが検査対象）。
- phase-4 以外のフェーズでのルール適用。
- `require:` に `human_gate` 以外の値を指定すること。
- ユーザーレベル / グローバルの instruction ファイル。

## 関連ドキュメント

- Design: `docs/superpowers/specs/2026-04-10-workflow-instructions-design.md`（英語のみ）
- Agent integration: `agents/task-decomposer.md`（英語のみ）
- Engine entry point: `mcp-server/internal/tools/pipeline_report_result.go`
