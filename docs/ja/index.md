---
layout: home

hero:
  name: claude-forge
  text: AI開発のためのパイプライン自動化
  tagline: アドホックなAIワークフローを、構造化されたマルチフェーズパイプラインに置き換えるClaude Codeプラグイン。隔離されたサブエージェント、決定論的ガードレール、再起動に耐える状態管理。
  actions:
    - theme: brand
      text: はじめる
      link: /ja/guide/introduction
    - theme: alt
      text: GitHub で見る
      link: https://github.com/hiromaily/claude-forge

features:
  - title: 工数に応じたスケーリング
    details: 工数レベル（S/M/L）に応じて3つのフローテンプレートから選択。軽量パイプラインからフルフェーズ実行まで。
    icon: ⚡
  - title: 決定論的ガードレール
    details: 重要な制約をシェルレベルのフックで強制。プロンプト指示だけに頼らない。読み取り専用ガード、コミットブロック、チェックポイントゲート。
    icon: 🛡️
  - title: AIレビューループ
    details: 設計とタスク計画は、実装開始前に専用レビューエージェントによるAPPROVE/REVISEサイクルを経る。
    icon: 🔄
  - title: ディスクベースの状態管理
    details: すべての進捗をstate.jsonでGo MCPサーバー（44ツール）を介して追跡。コンテキスト圧縮やセッション再起動に耐える。
    icon: 💾
  - title: 10の専門エージェント
    details: 各フェーズを専用エージェントが独立したコンテキストウィンドウで処理。状態共有なし、コンテキスト汚染なし。
    icon: 🤖
  - title: 改善レポート
    details: 毎回の実行後にドキュメントギャップ、フリクションポイント、トークン集中フェーズを特定する振り返りを自動出力。
    icon: 📊
---
