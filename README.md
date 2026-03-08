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
- Discord VC の参加、音声 transcript 保存、Realtime 音声 session を持つ
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
- `OPENAI_API_KEY`

Codex 認証は bot 専用 `runtime/codex-home/` で行う。

```bash
CODEX_HOME=./runtime/codex-home codex login
```

VC 音声の encode / decode には `libopus` が必要。

## DAVE Setup

Discord VC の E2EE/DAVE 検証には `Sigumaa/discordgo` fork と `libdave` が必要。

```bash
./scripts/setup-discordgo-dave.sh
```

この script は `any/discordgo/` に fork を clone し、upstream 由来の `setup-dave.sh` を実行し、`go.work` で local replace を作る。

`Sigumaa/discordgo` 自体には `libdave` の build artifact を含めない。初回は local clone 上で `setup-dave.sh` を実行する前提。
