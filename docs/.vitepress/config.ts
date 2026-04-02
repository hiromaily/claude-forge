import { defineConfig } from "vitepress";
import { withMermaid } from "vitepress-plugin-mermaid";

export default withMermaid(
  defineConfig({
    title: "claude-forge",
    description:
      "A Claude Code plugin that orchestrates multi-phase development pipelines with isolated subagents",

    base: "/claude-forge/",
    lastUpdated: true,
    cleanUrls: true,

    head: [["link", { rel: "icon", type: "image/svg+xml", href: "/claude-forge/logo.svg" }]],

    locales: {
      root: {
        label: "English",
        lang: "en",
        themeConfig: {
          nav: [
            { text: "Guide", link: "/guide/introduction" },
            { text: "Architecture", link: "/architecture/overview" },
            { text: "Agents", link: "/agents/overview" },
            { text: "Reference", link: "/reference/mcp-tools" },
          ],
          sidebar: {
            "/guide/": [
              {
                text: "Getting Started",
                items: [
                  { text: "Introduction", link: "/guide/introduction" },
                  { text: "Installation", link: "/guide/installation" },
                  { text: "Quick Start", link: "/guide/quick-start" },
                ],
              },
              {
                text: "Core Concepts",
                items: [
                  { text: "Pipeline Flow", link: "/guide/pipeline-flow" },
                  { text: "Flow Templates", link: "/guide/flow-templates" },
                  {
                    text: "Human Interaction Points",
                    link: "/guide/human-interaction",
                  },
                ],
              },
            ],
            "/architecture/": [
              {
                text: "Architecture",
                items: [
                  { text: "Overview", link: "/architecture/overview" },
                  {
                    text: "Design Principles",
                    link: "/architecture/design-principles",
                  },
                  {
                    text: "Hooks & Guardrails",
                    link: "/architecture/hooks",
                  },
                  {
                    text: "State Management",
                    link: "/architecture/state-management",
                  },
                ],
              },
            ],
            "/agents/": [
              {
                text: "Agents",
                items: [
                  { text: "Overview", link: "/agents/overview" },
                  {
                    text: "situation-analyst",
                    link: "/agents/situation-analyst",
                  },
                  { text: "investigator", link: "/agents/investigator" },
                  { text: "architect", link: "/agents/architect" },
                  {
                    text: "design-reviewer",
                    link: "/agents/design-reviewer",
                  },
                  {
                    text: "task-decomposer",
                    link: "/agents/task-decomposer",
                  },
                  { text: "task-reviewer", link: "/agents/task-reviewer" },
                  { text: "implementer", link: "/agents/implementer" },
                  { text: "impl-reviewer", link: "/agents/impl-reviewer" },
                  {
                    text: "comprehensive-reviewer",
                    link: "/agents/comprehensive-reviewer",
                  },
                  { text: "verifier", link: "/agents/verifier" },
                ],
              },
            ],
            "/reference/": [
              {
                text: "Reference",
                items: [
                  { text: "MCP Tools", link: "/reference/mcp-tools" },
                  { text: "CLI Flags", link: "/reference/cli-flags" },
                  {
                    text: "Environment Variables",
                    link: "/reference/env-vars",
                  },
                ],
              },
            ],
          },
        },
      },
      ja: {
        label: "日本語",
        lang: "ja",
        link: "/ja/",
        themeConfig: {
          nav: [
            { text: "ガイド", link: "/ja/guide/introduction" },
            { text: "アーキテクチャ", link: "/ja/architecture/overview" },
            { text: "エージェント", link: "/ja/agents/overview" },
            { text: "リファレンス", link: "/ja/reference/mcp-tools" },
          ],
          sidebar: {
            "/ja/guide/": [
              {
                text: "はじめに",
                items: [
                  { text: "概要", link: "/ja/guide/introduction" },
                  { text: "インストール", link: "/ja/guide/installation" },
                  {
                    text: "クイックスタート",
                    link: "/ja/guide/quick-start",
                  },
                ],
              },
              {
                text: "コアコンセプト",
                items: [
                  {
                    text: "パイプラインフロー",
                    link: "/ja/guide/pipeline-flow",
                  },
                  {
                    text: "フローテンプレート",
                    link: "/ja/guide/flow-templates",
                  },
                  {
                    text: "ヒューマンインタラクション",
                    link: "/ja/guide/human-interaction",
                  },
                ],
              },
            ],
            "/ja/architecture/": [
              {
                text: "アーキテクチャ",
                items: [
                  { text: "概要", link: "/ja/architecture/overview" },
                  {
                    text: "設計原則",
                    link: "/ja/architecture/design-principles",
                  },
                  {
                    text: "Hooks & ガードレール",
                    link: "/ja/architecture/hooks",
                  },
                  {
                    text: "状態管理",
                    link: "/ja/architecture/state-management",
                  },
                ],
              },
            ],
            "/ja/agents/": [
              {
                text: "エージェント",
                items: [
                  { text: "概要", link: "/ja/agents/overview" },
                  {
                    text: "situation-analyst",
                    link: "/ja/agents/situation-analyst",
                  },
                  { text: "investigator", link: "/ja/agents/investigator" },
                  { text: "architect", link: "/ja/agents/architect" },
                  {
                    text: "design-reviewer",
                    link: "/ja/agents/design-reviewer",
                  },
                  {
                    text: "task-decomposer",
                    link: "/ja/agents/task-decomposer",
                  },
                  {
                    text: "task-reviewer",
                    link: "/ja/agents/task-reviewer",
                  },
                  { text: "implementer", link: "/ja/agents/implementer" },
                  {
                    text: "impl-reviewer",
                    link: "/ja/agents/impl-reviewer",
                  },
                  {
                    text: "comprehensive-reviewer",
                    link: "/ja/agents/comprehensive-reviewer",
                  },
                  { text: "verifier", link: "/ja/agents/verifier" },
                ],
              },
            ],
            "/ja/reference/": [
              {
                text: "リファレンス",
                items: [
                  { text: "MCPツール", link: "/ja/reference/mcp-tools" },
                  { text: "CLIフラグ", link: "/ja/reference/cli-flags" },
                  { text: "環境変数", link: "/ja/reference/env-vars" },
                ],
              },
            ],
          },
          outline: { label: "目次" },
          lastUpdated: { text: "最終更新" },
          docFooter: { prev: "前のページ", next: "次のページ" },
          returnToTopLabel: "トップに戻る",
          sidebarMenuLabel: "メニュー",
          darkModeSwitchLabel: "ダークモード",
        },
      },
    },

    themeConfig: {
      socialLinks: [
        {
          icon: "github",
          link: "https://github.com/hiromaily/claude-forge",
        },
      ],
      search: { provider: "local" },
      footer: {
        message: "Released under the MIT License.",
        copyright: "Copyright © 2025-present",
      },
    },

    mermaid: {},
  }),
);
