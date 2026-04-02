# task-reviewer

**フェーズ:** 4b — タスクレビュー

## 役割

タスクリストのクリティカルな品質ゲート。設計カバレッジ、削除タスク、テスト更新、依存関係の正確性、並列安全性、受入基準の具体性を検証します。

## 入力

- `tasks.md` — Phase 4 のタスクリスト

## 出力

- `review-tasks.md` — 判定（APPROVE、APPROVE_WITH_NOTES、または REVISE）と指摘事項

## 制約

- `bugfix`、`investigation`、`docs` タスクタイプではスキップ
- 工数 L（`full` テンプレート）でのみ実行

## 判定

| 判定 | 意味 | パイプラインアクション |
| --- | --- | --- |
| **APPROVE** | タスクは準備完了 | チェックポイント B へ進む |
| **APPROVE_WITH_NOTES** | 軽微な問題 | オーケストレーターがインライン修正 |
| **REVISE** | 重大な指摘事項 | Phase 4（task-decomposer）を再実行 |
