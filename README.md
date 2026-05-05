# vrchat-safe-ranking-system

## セットアップ

```bash
cp .env.example .env
# .env を開いて JWT_SECRET / HMAC_SAVE_SECRET / HMAC_LOAD_SECRET を長い乱数文字列に差し替え
mkdir -p data
go build ./...
```

## 動作確認 (Bot 不要)

```bash
# 1) API 起動
go run ./cmd/api

# 2) 別ターミナルで E2E
go run ./cmd/vrcsim e2e --name alice --score 1234
```

期待される出力:

```
=> challenge
   UUID: 550e8400-...
=> register (DB direct, bypassing bot)
   JWT: eyJhbGciOi...
=> save (with JWT)
   success
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

# セーブ送信 (HMAC 計算 + JWT 込み)
go run ./cmd/vrcsim save --score 9999 --jwt 'eyJ...'

# ロード (JWT 必須; レスポンスの sig をクライアント側で検証する)
go run ./cmd/vrcsim load --jwt 'eyJ...'

# ランキング
curl 'http://localhost:8100/ranking?limit=10'

# URL を表示するだけ (リクエストしない、Udon TextBox 確認用)
go run ./cmd/vrcsim save --score 100 --jwt 'eyJ...' --print-url
go run ./cmd/vrcsim load --jwt 'eyJ...' --print-url
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
| `/unregister` | ランキングから自分を除外する (DisplayName 予約は維持。再度 `/register` で復帰可能) |
| `/ban discord_id:<id> reason:<...>` | 管理者専用。ランキング非掲載 + 登録不可 |
| `/unban discord_id:<id>` | 管理者専用 |
| `/invalidate-token jti:<...>` | 管理者専用。JTI 単体の無効化 |
| `/whois name:<DisplayName>` | 管理者専用。VRChat 名から登録情報を検索 |
| `/whois user:<@mention>` | 管理者専用。Discord ユーザーから登録情報を検索 |
| `/release-name name:<DisplayName>` | 管理者専用。他のアカウントに不正取得された DisplayName を強制解放 |

## エンドポイント仕様 (Udon 実装者向け)

すべて GET。`/load` のレスポンスのみ JSON、他は `text/plain` (`/ranking` も JSON)。

### `GET /challenge?name=<DisplayName>`

- レスポンス: UUID 文字列 1 行
- レート: 同 DisplayName について 1 分に 1 回 (`429`)
- TTL: UUID は 5 分で失効 (`/register` 時に消費)

### `GET /save?score=<int>&jwt=<JWT>&sig=<hex>`

- `jwt` は **必須**。JWT に含まれる DisplayName でランキングを紐付ける
- `sig` = `HMAC-SHA256(HMAC_SAVE_SECRET, "<score>")` → lowercase hex
- JWT が無効または JTI がブラックリスト済みの場合は `401`
- レスポンス: `success`

### `GET /load?jwt=<JWT>`

- `jwt` は **必須**。JWT に含まれる DisplayName の最新スコアを返す
- リクエストに sig は不要
- セーブなしの場合は `404`
- **レスポンス (200)**: JSON

```json
{"score": 1234, "sig": "<hex>"}
```

`sig` = `HMAC-SHA256(HMAC_LOAD_SECRET, "<score>")` → lowercase hex

Udon クライアント側でこの sig を検証することで MITM によるスコア改ざんを検知できる。

### `GET /ranking?limit=10`

検証用。有効な JWT でセーブ済み・BAN されていないユーザーを JSON 配列で返す。

## HMAC 対象文字列フォーマット

| 場面 | メッセージ | 鍵 | 方向 |
| --- | --- | --- | --- |
| `/save` リクエスト sig | `<score>` | `HMAC_SAVE_SECRET` | クライアント → サーバー |
| `/load` レスポンス sig | `<score>` | `HMAC_LOAD_SECRET` | サーバー → クライアント |

`score` は 10 進整数文字列 (Go の `strconv.FormatInt(v, 10)` 相当)。

## 既知の制約

- HTTPS は未対応 (本番は Caddy/nginx 前段か `crypto/tls`)
- スコアは整数 1 値のみ
- Udon クライアントに埋め込んだ HMAC 秘密鍵は抽出可能。`HMAC_SAVE_SECRET` が漏洩すると他者のスコアを偽装できるが、JWT 必須化により Discord アカウントと紐付いた正規ユーザーしか書き込めない構造になっている
- チケット発行時の DisplayName 所有確認なし → レート制限のみで対応
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
