# design-reviewer

**フェーズ:** 3b — 設計レビュー

## 役割

設計のクリティカルな品質ゲート。カバレッジ、完全性、一貫性、テスト戦略、矛盾、スコープクリープをレビューします。

## 入力

- `design.md` — Phase 3 の設計ドキュメント

## 出力

- `review-design.md` — 判定（APPROVE、APPROVE_WITH_NOTES、または REVISE）と指摘事項

## 制約

- `bugfix`、`docs`、`refactor` タスクタイプではスキップ

## 判定

| 判定 | 意味 | パイプラインアクション |
| --- | --- | --- |
| **APPROVE** | 設計は準備完了 | チェックポイント A へ進む |
| **APPROVE_WITH_NOTES** | 軽微な問題のみ | オーケストレーターがインライン修正、再レビュー |
| **REVISE** | 重大な指摘事項 | Phase 3（architect）を再実行 |

指摘事項は CRITICAL または MINOR に分類されます。CRITICAL な指摘事項のみが REVISE 判定をトリガーします。
