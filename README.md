# vrchat-safe-ranking-system

## セットアップ

```bash
cp .env.example .env
# .env を開いて以下を設定:
#   JWT_SECRET / HMAC_SAVE_SECRET / HMAC_LOAD_SECRET  — 16バイト以上の乱数
#   DISCORD_CLIENT_ID / DISCORD_CLIENT_SECRET         — Discord アプリの OAuth2 クレデンシャル
#   BASE_URL                                          — 公開 URL
#   OAUTH_REDIRECT_URL                                — 省略時 <BASE_URL>/auth/callback
mkdir -p data
go build ./...
```

Discord 開発者ポータルでアプリを作成し、OAuth2 リダイレクト URI に
`<BASE_URL>/auth/callback` を登録すること。スコープは `identify` のみで足りる。

## 動作確認 (Discord OAuth なし)

```bash
# 1) API 起動
go run ./cmd/api

# 2) 別ターミナルで E2E (registration.Service を直接叩いてOAuthをバイパス)
go run ./cmd/vrcsim e2e --name alice --score 1234
```

期待される出力:

```sh
=> register (DB direct, bypassing Discord OAuth)
   JWT: eyJhbGciOi...
=> save
   success
=> load
   score = 1234
=> ranking (top 10)
   #1 alice : 1234
```

## 動作確認 (Mock OAuth — Discord アプリなしで OAuth 風フロー)

`.env` に `OAUTH_MODE=mock` を設定するとブラウザフローは以下の3段リダイレクトに置き換わる:

```
/auth/start?name=alice&fake_discord_id=100000000000000001&fake_username=alice.dev
   ↓ 302
/auth/mock-login?state=...&discord_id=100000000000000001&username=alice.dev   (Discord 認可画面の代わり)
   ↓ 302
/auth/callback?code=100000000000000001|alice.dev&state=...
   ↓ 200 HTML (ポータル画面: @username + 現状表示 + アクションボタン)
```

どちらのクエリも optional:

- `fake_discord_id` 省略時はリクエストごとに 18 桁ランダム snowflake を生成
- `fake_username` 省略時は `name` クエリの値をそのまま流用

```bash
# .env: OAUTH_MODE=mock を設定
go run ./cmd/api

# 最小: name だけで十分。ID は乱数、username は alice として表示される
xdg-open 'http://localhost:8100/auth/start?name=alice'

# 同じ ID で再アクセスしたい場合は明示的に指定
xdg-open 'http://localhost:8100/auth/start?name=alice&fake_discord_id=100000000000000001'

# username だけ Discord 風に変えたい場合
xdg-open 'http://localhost:8100/auth/start?name=alice&fake_username=alice.dev'
```

## 個別動作確認

```bash
# ブラウザで認証開始 (Discord モード)
'http://localhost:8100/auth/start?name=alice'

# セーブ (HMACは自動計算)
go run ./cmd/vrcsim save --score 9999 --jwt 'eyJ...'

# ロード (レスポンスは自動検証)
go run ./cmd/vrcsim load --jwt 'eyJ...'

# ランキング
curl 'http://localhost:8100/ranking?limit=10'

# URL を表示するだけ (リクエストしない、Udon TextBox 確認用)
go run ./cmd/vrcsim save --score 100 --jwt 'eyJ...' --print-url
go run ./cmd/vrcsim load --jwt 'eyJ...' --print-url
```

## 管理 CLI

ban / unban / whois / DisplayName 解放 / JTI 無効化はサーバホスト上の CLI で行う。
DB ファイル (`DB_PATH`) に直接アクセスするので、API と同じホストで実行すること。

```bash
go run ./cmd/admin ban --discord-id 123456789012345678 --reason 不正
go run ./cmd/admin unban --discord-id 123456789012345678
go run ./cmd/admin whois --name alice
go run ./cmd/admin whois --discord-id 123456789012345678
go run ./cmd/admin release-name --name bob
go run ./cmd/admin invalidate-token --jti <jti>
```

## エンドポイント仕様

### OAuth フロー (ブラウザ)

| Method | Path                                  | 用途                                                  |
| ------ | ------------------------------------- | ----------------------------------------------------- |
| GET    | `/auth/start?name=<DisplayName>`      | Discord OAuth を開始 (action 不要、name のみ必須)     |
| GET    | `/auth/callback?code=&state=`         | Discord からの戻り。state を検証し portal-view へ 303 |
| GET    | `/auth/portal?token=<x>`              | ポータル画面のレンダリング (consume なし、リロード可) |
| POST   | `/auth/portal`                        | アクションボタンの確定。`token`/`action` をフォーム送信 |

`/auth/start?name=alice` は CSRF 用の state を発行し Discord 認可画面に 302 する。
ユーザーが認可すると `/auth/callback` に戻り、state を消費しつつ単発使用のセッショントークンを発行、
`GET /auth/portal?token=…` に **303 リダイレクト**する。
リダイレクト先で表示される**ポータル画面**には:

- 認証された Discord ID
- 現在の登録状態 (登録済みなら DisplayName と現在の JWT を直接表示)
- 文脈に応じたアクションボタン:
  - 未登録: `[Register]`
  - 登録済み・同名: `[Reissue token]` `[Unregister]`
  - 登録済み・改名: `[Apply name change]` `[Unregister]`
  - name が他アカウント所有: 警告のみ表示 (Register は出さない)

トークン確認 (mytoken) はポータル冒頭に常に表示されるため別アクションは不要。
`[Register]` `[Unregister]` 押下時のみ `POST /auth/portal` が叩かれてコミットされる。
ポータル画面自体が「いま誰として何が起こるか」の確認ステップを兼ねる。

ポータルは GET (view) と POST (commit) で分離されているため、ブラウザの戻る/リロードで
`/auth/portal?token=…` に再アクセスしても session 期限内なら何度でも同じ画面が出る。
`/auth/callback?state=…&code=…` 自体のリロードだけは OAuth 仕様上単発使用なので失敗する。

これにより、誤って別の Discord アカウント (サブ垢など) で OAuth した場合でも
ポータルで気付いて中断でき、本垢の JWT が無断で取り消される事故を防げる。
さらに、本垢の name で OAuth しようとしてもサブ垢でログインしていれば
「name は他アカウント所有」警告で Register 自体出ないため、二重に守られる。

### Udon クライアント向け (GET のみ)

セーブデータは `internal/savedata.Data` 構造体で表現される JSON オブジェクト。
現状のフィールドは `score` のみ。フィールドを追加する際は

1. `internal/savedata/savedata.go` の `Data` 構造体に **末尾追加** で field を生やす
2. `save_history` / `latest_saves` に対応するカラムを `ALTER TABLE ADD COLUMN` で追加
3. `internal/db/saves.go` の `Save` の INSERT と `scanSaveEntry` の SELECT を拡張

の3点をセットで行う。HMAC 入力はこの JSON のキャノニカルバイト列。

#### `GET /save?data=<urlencoded JSON>&jwt=<JWT>&sig=<hex>`

- `data` は `savedata.Data` をシリアライズした JSON (例: `{"score":1234}`)。
  URL クエリに載せる際は URL エンコードする
- `sig` = `HMAC-SHA256(HMAC_SAVE_SECRET, <data の URL デコード後 raw bytes>)` → lowercase hex
- `jwt` は **必須**。JWT に含まれる DisplayName でランキングを紐付ける
- JWT が無効または JTI がブラックリスト済みの場合は `401`
- レスポンス: `success`

#### `GET /load?jwt=<JWT>`

- `jwt` は **必須**。JWT に含まれる DisplayName の最新セーブを返す
- リクエストに sig は不要
- セーブなしの場合は `404`
- **レスポンス (200)**: JSON

```json
{"data":{"score":1234},"sig":"<hex>"}
```

`sig` = `HMAC-SHA256(HMAC_LOAD_SECRET, <data の raw JSON bytes>)` → lowercase hex

Udon クライアント側でこの sig を検証することで MITM による改ざんを検知できる。
`data` フィールドは server がエンコードしたバイト列をそのまま受け取り (空白なし、
field 順は構造体定義順) HMAC を計算する想定。

#### `GET /ranking?limit=10`

検証用。有効な JWT でセーブ済み・BAN されていないユーザーを JSON 配列で返す。

## HMAC 対象バイト列フォーマット

| 場面                   | メッセージ                       | 鍵                 | 方向                    |
| ---------------------- | -------------------------------- | ------------------ | ----------------------- |
| `/save` リクエスト sig | `data` の URL デコード後 raw bytes | `HMAC_SAVE_SECRET` | クライアント → サーバー |
| `/load` レスポンス sig | レスポンス内 `data` の raw bytes   | `HMAC_LOAD_SECRET` | サーバー → クライアント |

両方向で同じ `savedata.Data` 構造体のキャノニカル JSON 表現が HMAC 入力となる。
Go の `encoding/json` 出力 (空白なし、struct 定義順) を canonical と定義する。

## 既知の制約

- HTTPS は未対応 (本番は Caddy/nginx 前段か `crypto/tls`)
- スコアは整数 1 値のみ
- Udon クライアントに埋め込んだ HMAC 秘密鍵は抽出可能。`HMAC_SAVE_SECRET` が漏洩
  すると他者のスコアを偽装できるが、JWT 必須化により Discord アカウントと紐付いた
  正規ユーザーしか書き込めない構造になっている
- マルチランキング非対応
- API と admin CLI は同一ホスト前提 (SQLite ファイル共有)

## ディレクトリ構成

```
cmd/api      — HTTP サーバ (Udon API + OAuth web flow)
cmd/admin    — 管理者 CLI (ban / whois / 等)
cmd/vrcsim   — VRChat Udon クライアント模擬 CLI
internal/api          — HTTP ハンドラ
internal/auth         — JWT (HS256) / HMAC ヘルパ
internal/config       — env 読み込み
internal/db           — SQLite スキーマ + リポジトリ
internal/oauth        — Discord OAuth Provider + テスト用 Fake
internal/registration — JWT 発行コア (OAuth callback / e2e CLI 共通)
internal/vrcclient    — Udon が叩く URL の組み立て + GET 実行
```
