パイプラインは工数レベルに基づいて実行を適応させます。オーケストレーターはワークスペースセットアップ時に `skip-phase` コマンドを使用して非対象フェーズを事前にスキップするため、最初の実際のフェーズが始まる前に `currentPhase` はすでにすべてのスキップ済みフェーズを通過した位置を指しています。

## 工数レベルとフェーズスキップテーブル

3つの工数レベルがサポートされています。`L` はフルパイプラインを実行します。低いレベルはフェーズをスキップします：

| 工数 | テンプレート | スキップするフェーズ |
|--------|----------|----------------|
| `S` | `light` | `phase-4b`, `checkpoint-b`, `phase-7` |
| `M` | `standard` | `phase-4b`, `checkpoint-b` |
| `L` | `full` | （なし） |

**工数レベルごとの根拠：**

- **`S`（light）**: タスクレビューの品質ゲート（`phase-4b`、`checkpoint-b`）と包括的レビュー（`phase-7`）をスキップします。タスク分解が単純で、実装後の包括的なレビューが不要な小規模・焦点の絞られたタスクに適しています。
- **`M`（standard）**: タスクレビューの品質ゲートのみをスキップします。Phase 7（包括的レビュー）は実行されます。実装レビューが価値を持つが、タスクの内訳が個別の品質ゲートを必要とするほど複雑ではない中規模フィーチャーに適しています。
- **`L`（full）**: 両チェックポイントと包括的レビューを含むすべてのフェーズが実行されます。すべての品質ゲートが価値を加える大規模・複雑なタスクに適しています。

## state.json スキーマの追加

初期 v1 スキーマを超えて、`state.json` にいくつかのトップレベルフィールドが追加されています：

```json
{
  "version": 1,
  "effort": "S | M | L | null",
  "flowTemplate": "light | standard | full | null",
  "skippedPhases": ["phase-4b", "checkpoint-b", "phase-7"],
  "autoApprove": false,
  "phaseLog": [
    {"phase": "phase-1", "tokens": 5000, "duration_ms": 30000, "model": "sonnet", "timestamp": "..."}
  ],
  ...
}
```

- `effort` はワークスペースセットアップ時に設定されるまで `null` です。`mcp__forge-state__set_effort` で設定されます。有効な値：`S`、`M`、`L`（XS はサポートされていません）。
- `flowTemplate` はワークスペースセットアップ時に設定されるまで `null` です。`mcp__forge-state__set_flow_template` で設定されます。有効な値：`light`、`standard`、`full`。再開の一貫性を保証するために状態に保存されます（再導出はしません）。
- `skippedPhases` は設定されるまで `[]` です。`skip-phase` の各呼び出しがこの配列に1つのフェーズ ID を追加します。
- `autoApprove` はデフォルトで `false` です。`--auto` フラグが存在する場合に `set-auto-approve` で設定されます。
- `phaseLog` は `phase-log` 経由でフェーズごとのメトリクス（トークン、期間、モデル）を記録します。`phase-stats` および最終サマリーの実行統計テーブルで使用されます。
- `version` は `1` のままです — 古い状態ファイルにはこれらのフィールドが単純に存在せず、オーケストレーターは `resume-info` のデフォルト値を通じて、不在を `null`/`[]`/`false` として扱います。

**不変条件：** `completedPhases` と `skippedPhases` は互いに排他的です。フェーズ ID はこれらの配列のどちらか一方にのみ現れます。`phase-complete` は `completedPhases` に追加し、`skip-phase` は `skippedPhases` に追加します。どちらのコマンドも相手の配列を変更しません。

## `skip-phase` コマンド vs `phase-complete`

`phase-complete` と `skip-phase` はどちらも `currentPhase` を正規 PHASES 配列の次のエントリに進めるメカニズムです。両者はその意味的な意味と副作用において異なります：

| 側面 | `phase-complete` | `skip-phase` |
|--------|-----------------|--------------|
| 意味 | フェーズが正常に実行された | フェーズが意図的にバイパスされた |
| 記録先 | `completedPhases` | `skippedPhases` |
| `currentPhase` の進行 | はい、`next_phase()` 経由 | はい、同じ `next_phase()` ロジック経由 |
| `currentPhaseStatus` の設定 | 次のフェーズに `"pending"` | 次のフェーズに `"pending"` |
| 呼び出しタイミング | フェーズエージェントの完了後 | フェーズが実行される前のワークスペースセットアップ中 |

`skip-phase` は `phase-complete` と同じ `next_phase()` 順序ロジックを使用するため、同じ順序不変条件が適用されます：フェーズは正規 PHASES 配列の順序で、ギャップなしに一度に1つずつ処理されなければなりません。

## アップフロントスキップパターン

すべての `skip-phase` 呼び出しは、最初の実際のフェーズが始まる前に、正規 PHASES 配列の順序で**ワークスペースセットアップ時に事前に**行われます。これは以下を意味します：

1. オーケストレーターはワークスペースセットアップ中に `{effort}` を決定します。
2. `{workspace}` と `{effort}` を指定して `mcp__forge-state__set_effort` を呼び出します。
3. スキップテーブルの各フェーズについて（正規の順序で）、`{workspace}` と `<phase>` を指定して `mcp__forge-state__skip_phase` を呼び出します。
4. オーケストレーターが最初のフェーズブロックに到達するまでに、`currentPhase` はすでにすべてのスキップ済みフェーズを通過した位置を指しています。

オーケストレーターはまだ各フェーズブロックでスキップゲートを確認します — 工数レベルがそのフェーズをスキップするようにマップされている場合、`phase-start` を呼び出したりエージェントを起動したりせずに次のブロックに直接進みます。

## 工数検出の優先順位

オーケストレーターはワークスペースセットアップ中にこの優先順位で `{effort}` を検出します：

1. **明示的フラグ**: `$ARGUMENTS` の `--effort=<value>`（`request.md` に書き込む前に引数から削除；有効な値：`S`、`M`、`L`；`XS` は入力バリデーション時に拒否される）
2. **Jira ストーリーポイント**: フェッチした Jira 課題から `customfield_10016` を読み取る。不在、None、非数値、またはゼロの場合はフォールスルー。マッピング：SP ≤ 4 → S、SP ≤ 12 → M、SP > 12 → L。
3. **ヒューリスティック**: タスクの説明の複雑さから推論する。
4. **デフォルト**: `M`（安全なフォールバック — この機能がデプロイされる前に開始されたパイプラインの現在の動作に一致）

検出後に呼び出す：`$SM set-effort {workspace} {effort}`

## フローテンプレートの選択

工数レベル単独が状態に保存される `flowTemplate` 文字列を決定します。XS 工数はサポートされていません；サポートされる最小工数は S です。ルックアップ後に呼び出す：`$SM set-flow-template {workspace} {flow_template}`

| 工数 | テンプレート | スキップされるフェーズ |
|--------|----------|----------------|
| S | `light` | `phase-4b`, `checkpoint-b`, `phase-7` |
| M | `standard` | `phase-4b`, `checkpoint-b` |
| L | `full` | _（なし）_ |

新しい Go ヘルパー関数：
- `EffortToTemplate(effort string) string` — 工数をテンプレート名にマップする
- `SkipsForEffort(effort string) []string` — 指定した工数レベルの正規スキップリストを返す

### テンプレート定義

| テンプレート | 実行されるフェーズ | エージェント数 |
|----------|-----------|-------------|
| `light` | Phase 1 → Phase 2 → Phase 3 → Phase 3b → Checkpoint A → Phase 4 → Phase 5 → Phase 6 → Verification → PR | 5+ |
| `standard` | フルパイプライン（4b/checkpoint-b 以外のすべてのフェーズ、両チェックポイント） | 10+ |
| `full` | Standard ＋ すべてのチェックポイント必須（`--auto` でも auto-approve 無効） | 10+ |

### スキップセットの計算

任意のパイプライン実行のスキップセットは、工数レベルのみによって決定されます。スキップセットはワークスペースセットアップ時に正規 PHASES 配列の順序で `skip-phase` 呼び出しとして出力されます。オーケストレーターはリストを事前に計算します — ランタイムの再計算は不要です。

## 統合アーティファクト可用性

完了したパイプライン後に存在するワークスペースアーティファクトファイルの単一リファレンス。上記の工数からテンプレートへのテーブルとスキップセットから導出されます。

**凡例：** `✓` エージェント生成 · `S` オーケストレータースタブ · `—` 生成されない

`summary.md` は常に生成されるため、テーブルから省略されています。

| 工数 | テンプレート | `analysis.md` | `investigation.md` | `design.md` | `review-design.md` | `tasks.md` | `review-tasks.md` | `impl-{N}.md` | `review-{N}.md` | `comprehensive-review.md` |
|--------|----------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| S | `light` | ✓ | ✓ | ✓ | ✓ | ✓ | — | ✓ | ✓ | — |
| M | `standard` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| L | `full` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

## 再開の動作

再開時、オーケストレーターは `resume_info.effort` から `{effort}` を、`resume_info.flowTemplate` から `{flow_template}` を、`resume_info.skippedPhases` から `{skipped_phases}` を復元します。フォールバックルール：

- `effort` が null の場合（工数フローがデプロイされる前に開始されたパイプライン）：**コンテキスト内のみで** `M` をデフォルトとし、メモを記録する。`set-effort` を呼び出さない — 状態にすでに記録されている `skippedPhases` が権威を持つ。
- `flowTemplate` が null の場合：`EffortToTemplate` を使用して工数から再導出し、**コンテキスト内のみで**保存する。`set-flow-template` を呼び出さない — 元の `skippedPhases` が権威を持つ。
- 再開したパイプラインの実行期間中、`{effort}` と `{flow_template}` をコンテキスト変数として保持する。
