# yururi_personal

Discord 上で動くパーソナル AI Agent `ゆるり` の実装。

## Overview

- Go 製の単一バイナリ
- Discord Bot API を直接利用
- Codex CLI App Server を子プロセスとして利用
- 通常会話は direct text turn で tool call を回し、必要なら途中経過も複数回送る
- bot 専用 `CODEX_HOME` と bot 専用 workspace を分離
- チャンネルごとの会話 thread、短期記憶、長期記憶、定期ジョブ、自律 pulse を内包
- Discord 管理、channel profile、URL 取得、URL 監視、background Codex task、画像添付読込の tool を持つ
- 起動時にチャンネルやカテゴリを自動作成しない
- `runtime/workspace/context/` に bot 向けの実能力と振る舞い方針を生成する

## Commands

```bash
go run ./cmd/yururi bootstrap
go run ./cmd/yururi serve
go run ./cmd/yururi reset
go run ./cmd/yururi reset --full
```

`reset` は runtime の DB、workspace、logs、Codex state を初期化する。`auth.json` は残す。`--full` は runtime 全体を作り直し、認証も消す。

## Runtime Layout

起動時に `runtime/` 配下へ以下を生成する。

- `runtime/codex-home/`
- `runtime/workspace/`
- `runtime/data/`
- `runtime/logs/`

## Configuration

アプリ設定は `config/example.toml` を元に作成する。

主な環境変数:

- `YURURI_DISCORD_TOKEN`
- `YURURI_GUILD_ID`
- `YURURI_OWNER_USER_ID`
- `YURURI_RUNTIME_ROOT`

Codex 認証は bot 専用 `runtime/codex-home/` で行う。

```bash
CODEX_HOME=./runtime/codex-home codex login
```
