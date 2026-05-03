# vrchat-safe-ranking-system

## セットアップ

```bash
cp .env.example .env
# .env を開いて 3 つの SECRET を長い乱数文字列に差し替え
mkdir -p data
go build ./...
```

## 動作確認 (Bot 不要)

```bash
# 1) API 起動
go run ./cmd/api

# 2) 別ターミナルで E2E
go run ./cmd/vrcsim e2e --name alice --discord-id 123456789012345678 --score 1234
```

期待される出力:

```
=> challenge
   UUID: 550e8400-...
=> register (DB direct, bypassing bot)
   JWT: eyJhbGciOi...
=> save (with JWT)
    OK ranked
=> load
   score = 1234
=> ranking (top 10)
   #1 alice : 1234
```

## 個別動作確認

```bash
# チケット発行
go run ./cmd/vrcsim challenge --name alice

# Bot 起動済みなら Discord で /register <UUID>
# Bot なしなら e2e サブコマンドで代替

# セーブ送信 (HMAC 計算込み)
go run ./cmd/vrcsim save --name alice --score 9999 --jwt 'eyJ...'

# JWT なしのローカルセーブ (新規ユーザの初回セーブ等)
go run ./cmd/vrcsim save --name alice --score 100

# ロード
go run ./cmd/vrcsim load --name alice

# ランキング
curl 'http://localhost:8100/ranking?limit=10'

# URL を表示するだけ (リクエストしない、Udon TextBox 確認用)
go run ./cmd/vrcsim save --name alice --score 100 --print-url
```

## Bot 起動

```bash
# .env に BOT_TOKEN, BOT_GUILD_ID, ADMIN_USER_IDS を設定
go run ./cmd/bot
```

スラッシュコマンド:

| コマンド | 説明 |
| --- | --- |
| `/register uuid:<UUID>` | UUID を JWT に引き換え。再実行で新しい JWT に更新 (旧 jti は自動的にブラックリスト入り) |
| `/mytoken` | 直近発行の JWT を再表示 |
| `/ban discord_id:<id> reason:<...>` | 管理者専用 |
| `/unban discord_id:<id>` | 管理者専用 |
| `/invalidate-token jti:<...>` | 管理者専用 |

## エンドポイント仕様 (Udon 実装者向け)

すべて GET。レスポンスは `text/plain` (`/ranking` のみ JSON)。

### `GET /challenge?name=<DisplayName>`

- レスポンス: UUID 文字列 1 行
- レート: 同 DisplayName について 1 分に 1 回 (`429`)
- TTL: UUID は 5 分で失効 (`/register` 時に消費)

### `GET /save?user_id=<DisplayName>&score=<int>&jwt=<JWT?>&sig=<hex>`

- `sig` = `HMAC-SHA256(HMAC_SAVE_SECRET, "<user_id>|<score>")` → lowercase hex
- `jwt` 省略時はローカルセーブ扱い (ランキング非掲載)
- レスポンス例: `OK ranked` / `OK ranked (not best)` / `OK saved` / `OK saved (jwt invalid)`
- ランキング掲載時は score が現在の自己ベストより高い場合のみ更新

### `GET /load?user_id=<DisplayName>&sig=<hex>`

- `sig` = `HMAC-SHA256(HMAC_LOAD_SECRET, "<user_id>")`
- レスポンス: 整数 1 行 (セーブが存在しなければ `404`)

### `GET /ranking?limit=10`

検証用。ランキング掲載対象 (jwt 検証通過済) を JSON 配列で返す。

## HMAC 対象文字列フォーマット

| エンドポイント | メッセージ | 鍵 |
| --- | --- | --- |
| `/save` | `<user_id>|<score>` | `HMAC_SAVE_SECRET` |
| `/load` | `<user_id>` | `HMAC_LOAD_SECRET` |

`score` は 10 進整数文字列 (符号なし負数なし、Go の `strconv.FormatInt(v, 10)` 相当)。
将来 `data` クエリ追加時は `<user_id>|<score>|<base64data>` のように `|` 区切りで延長する想定。

## 既知の制約

- HTTPS は未対応 (本番は Caddy/nginx 前段か `crypto/tls`)
- スコアは整数 1 値のみ (任意 JSON `data` は本プロトでは扱わない)
- `/save` の HMAC 対象に `jwt` は含めない → JWT を差し替えての偽装余地はあるが、`claims.display_name == user_id` 検証で実害は限定 (改名対応の都合)
- チケット発行時の DisplayName 所有確認なし → 設計通りレート制限のみで対応
- マルチランキング非対応
- Bot/API は同一ホスト前提 (SQLite ファイル共有)。別ホストにする場合は PostgreSQL 等への差し替えが必要

## ディレクトリ構成

```
cmd/api      — HTTP サーバ
cmd/bot      — Discord Bot
cmd/vrcsim   — VRChat Udon クライアント模擬 CLI
internal/api          — HTTP ハンドラ
internal/auth         — JWT (HS256) / HMAC ヘルパ
internal/config       — env 読み込み
internal/db           — SQLite スキーマ + リポジトリ
internal/registration — /register フロー (bot/vrcsim 共有)
internal/vrcclient    — Udon が叩く URL の組み立て + GET 実行
```
