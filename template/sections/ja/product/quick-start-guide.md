## 基本的な使い方

プラグインがインストールされたClaude Codeセッションからスキルを呼び出します：

```text
/forge <タスクの説明>
/forge https://github.com/org/repo/issues/123
/forge https://myorg.atlassian.net/browse/PROJ-456
```

GitHub IssueまたはJira URLを指定すると、パイプラインはIssueの詳細を取得し、最終サマリーをコメントとして投稿します。

## 使用例

```text
# 小規模タスク、自動承認
/forge --effort=S --auto 認証ミドルウェアのnullポインタクラッシュを修正

# 中規模タスク、PR作成をスキップ
/forge --nopr APIクライアントにリトライロジックを追加

# 大規模タスク、デバッグ診断付き
/forge --effort=L --debug 新しいバリデーションレイヤーを追加
```

## フラグ

| フラグ | 説明 |
| --- | --- |
| `--effort=<S\|M\|L>` | 工数レベルを指定。フローテンプレート（light/standard/full）を決定。デフォルト：`M`。 |
| `--auto` | AIの判定がAPPROVEの場合、ヒューマンチェックポイントをスキップ。REVISEの場合は一時停止。 |
| `--nopr` | PR作成をスキップ。変更はコミット・プッシュされるがPRは作成されない。 |
| `--debug` | `summary.md` に実行診断のデバッグレポートを追加。 |
| _（自動検出）_ | specディレクトリ名を指定して再開。フラグ不要。 |

## 中断されたパイプラインの再開

specディレクトリ名を指定。`.specs/` ディレクトリの存在から自動検出されます：

```text
/forge 20260320-fix-auth-timeout
```

## パイプラインの中止

MCPツールを使用：

```text
mcp__forge-state__abandon with workspace: .specs/20260320-fix-auth-timeout
```

または状態ファイルを削除：

```bash
rm .specs/20260320-fix-auth-timeout/state.json
```

## 実行中に起こること

1. **入力バリデーション** — 決定論的 + セマンティックチェック
2. **ワークスペースセットアップ** — `.specs/` に `request.md` と `state.json` を作成
3. **分析フェーズ** — 状況分析と調査（読み取り専用）
4. **設計** — アーキテクトが `design.md` を作成、レビュアーが承認または修正
5. **ヒューマンチェックポイント** — 設計をレビュー
6. **タスク分解** — タスクを分割、レビュー
7. **実装** — 各タスクをTDDで実装、その後コードレビュー
8. **検証** — 包括的レビュー + 最終ビルド/テスト検証
9. **PR作成** — コミット、プッシュ、PR作成
10. **サマリー** — 改善レポート付き `summary.md`

詳細なフェーズの説明は [パイプラインフロー](/ja/guide/pipeline-flow) を参照してください。工数ベースのフェーズ選択は [フローテンプレート](/ja/guide/flow-templates) を参照してください。
