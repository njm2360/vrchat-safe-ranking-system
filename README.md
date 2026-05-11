# vrchat-safe-ranking-system

VRChatワールド用クラウドセーブ・ロード・ランキングAPI
Udonワールドは秘密鍵がデコンパイルで抜かれる前提があるため、
SipHash署名に加えてDiscord OAuth連携のJWTを併用し、データを保護する。

## 不正防止モデル

### 防御層

| 層                                              | 守るもの                                  | 突破に必要なもの                                  |
| ------------------------------------------------ | ----------------------------------------- | ------------------------------------------------- |
| SipHash 署名 (`SAVE_SECRET` / `LOAD_SECRET`)    | URLクエリ書き換え・ロード応答のMITM改ざん | Udon内に埋め込まれた鍵 (デコンパイル等で抽出可能) |
| SipHash 署名 (`AUTH_SECRET`)                    | OAuthフローでのユーザー名詐称登録         | Udon内に埋め込まれた鍵 (デコンパイル等で抽出可能) |
| JWT 認証 (Discord OAuthで発行)                  | 連携済みユーザー名への書き込み・読み出し  | JWTトークン (秘密鍵はサーバー)                    |

### 認証モデル

`/save` `/load` の `jwt` クエリはオプションで、サーバはユーザー名の連携状態で認証を切り替える

| ユーザー名の状態 | 必要な認証                                                                |
| ---------------- | ------------------------------------------------------------------------- |
| Discord未連携    | 署名のみ (従来どおりの仕様を維持)                                         |
| Discord連携済み  | 署名 + JWT (クレームのユーザー名一致 / 現行JTI / ブラックリスト未登録)    |

### トークン発行 = 自分のデータを守るアクション

ワールドアセットから署名鍵が漏洩しても、被害はDiscord未連携のユーザー名に限定される。
登録済みユーザー名はJWT検証が必須であるため、攻撃者が署名鍵だけでは上書きできない。
そのため、Discord連携してトークンを取得すること自体が自分のデータを守る防御アクションになる。
万が一トークンが漏れた場合はポータルからトークンを再発行すれば、旧トークンの書き込みは即弾かれる。

### ランキング表示の`verified`フラグ

`/ranking` の各エントリには `verified: bool` が付き、連携された認証済みユーザーかを区別できる。
`verified=true`クエリで認証済みエントリのみに絞ることも可能。

## セットアップ

```bash
cp .env.example .env
# .env を開いて以下を設定:
#   JWT_SECRET                                        — 16バイト以上の乱数
#   SAVE_SECRET / LOAD_SECRET / AUTH_SECRET           — 各ちょうど 16 バイトの鍵 (SipHash-2-4)
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

`.env` に `OAUTH_MODE=mock` を設定するとブラウザフローは以下に置き換わる:

```
/auth/start?display_name=alice&sig=<hex>&fake_discord_id=100000000000000001&fake_username=alice.dev
   ↓ 302
/auth/mock-login?state=...&discord_id=100000000000000001&username=alice.dev   (Discord 認可画面の代わりに HTML フォームを表示)
   ↓ POST /auth/mock-login (フォーム送信)
/auth/callback?code=100000000000000001|alice.dev&state=...
   ↓ 303 + Set-Cookie: vsrs_portal_session=…
/auth/portal
   ↓ 200 HTML (ポータル画面: @username + 現状表示 + アクションボタン)
```

`display_name` と `sig` は必須、それ以外は optional:

- `sig` = `SipHash-2-4(AUTH_SECRET, <display_name bytes>)` の lowercase hex (16文字)
- `fake_discord_id` 省略時はリクエストごとに 18 桁ランダム snowflake を生成
- `fake_username` 省略時は `display_name` クエリの値をそのまま流用

```bash
# .env: OAUTH_MODE=mock を設定
go run ./cmd/api

# 署名付き URL は vrcsim から取得 (sig を手で計算しなくて済む)
xdg-open "$(go run ./cmd/vrcsim auth-start-url --display-name alice)"

# 同じ ID で再アクセスしたい場合
xdg-open "$(go run ./cmd/vrcsim auth-start-url --display-name alice --fake-discord-id 100000000000000001)"

# username だけ Discord 風に変えたい場合
xdg-open "$(go run ./cmd/vrcsim auth-start-url --display-name alice --fake-username alice.dev)"
```

## 個別動作確認

```bash
# ブラウザで認証開始 (Discord モード) — sig は vrcsim が計算する
xdg-open "$(go run ./cmd/vrcsim auth-start-url --display-name alice)"

# セーブ (sig は自動計算)
go run ./cmd/vrcsim save --score 9999 --display-name alice --jwt 'eyJ...'

# ロード (レスポンスは自動検証)
go run ./cmd/vrcsim load --display-name alice --jwt 'eyJ...'

# ランキング
curl 'http://localhost:8100/ranking?limit=10'

# URL を表示するだけ (リクエストしない、Udon TextBox 確認用)
go run ./cmd/vrcsim save --score 100 --display-name alice --jwt 'eyJ...' --print-url
go run ./cmd/vrcsim load --display-name alice --jwt 'eyJ...' --print-url
```

## 管理 CLI

ban / unban / whois / DisplayName 解放 / JTI 無効化はサーバホスト上の CLI で行う。
DB ファイル (`DB_PATH`) に直接アクセスするので、API と同じホストで実行すること。

```bash
# Discord ID で BAN (BAN/UNBAN は --name でも指定可)
go run ./cmd/admin ban --discord-id 123456789012345678 --reason 不正
go run ./cmd/admin ban --name baduser --reason 不正
go run ./cmd/admin unban --discord-id 123456789012345678
go run ./cmd/admin unban --name baduser

go run ./cmd/admin whois --name alice
go run ./cmd/admin whois --discord-id 123456789012345678
go run ./cmd/admin release-name --name bob
go run ./cmd/admin invalidate-token --jti <jti>
```

## エンドポイント仕様

### OAuth フロー (ブラウザ)

| Method | Path                                               | 用途                                                                   |
| ------ | -------------------------------------------------- | ---------------------------------------------------------------------- |
| GET    | `/auth/start?display_name=<DisplayName>&sig=<hex>` | Discord OAuth を開始 (display_name と sig が必須)                      |
| GET    | `/auth/callback?code=&state=`                      | Discord からの戻り。state を消費しセッション Cookie を発行             |
| GET    | `/auth/portal`                                     | ポータル画面のレンダリング (Cookie 検証のみ、consume なし、リロード可) |
| POST   | `/auth/register`                                   | 登録 / トークン再発行 / 改名のコミット (セッション Cookie を消費)      |
| POST   | `/auth/unregister`                                 | 登録解除のコミット (セッション Cookie を消費)                          |

`/auth/start?display_name=alice&sig=…` は CSRF 用の state を発行し Discord 認可画面に 302 する。
`sig` は `SipHash-2-4(AUTH_SECRET, display_name)` の lowercase hex で、Udon ワールド内に
埋め込まれた鍵を持つクライアントのみが OAuth フローを起動できるようにする
ユーザーが認可すると `/auth/callback` に戻り、state を消費しつつ単発使用のセッション
Cookie (`vsrs_portal_session`, Path=`/auth`, HttpOnly) を発行して `/auth/portal` に
**303 リダイレクト**する。
リダイレクト先で表示される**ポータル画面**には:

- 認証された Discord ユーザー名 (`@username`)
- 現在の登録状態 (登録済みなら現行 DisplayName を表示。**JWT は表示されない**)
- 文脈に応じたアクションボタン:
  - 未登録: `[登録]`
  - 登録済み・同名: `[トークンを再発行]` `[登録解除]`
  - 登録済み・改名: `[ユーザー名を変更]` `[登録解除]`
  - name が他アカウント所有 / BAN 済: 警告のみ表示 (登録アクションは出さない)

`[登録]` 系を押すと `POST /auth/register`、`[登録解除]` を押すと `POST /auth/unregister`
が叩かれ、その時点で Cookie が消費される。Register コミット成功時のみ JWT が画面に
表示される (renderToken)。

ポータルは GET (view) と POST (commit) で分離されているため、ブラウザの戻る/リロードで
`/auth/portal` に再アクセスしても session 期限内なら何度でも同じ画面が出る。
`/auth/callback?state=…&code=…` 自体のリロードだけは OAuth 仕様上単発使用なので失敗する。

これにより、誤って別の Discord アカウント (サブ垢など) で OAuth した場合でも
ポータルで気付いて中断でき、本垢の JWT が無断で取り消される事故を防げる。
さらに、本垢の name で OAuth しようとしてもサブ垢でログインしていれば
「name は他アカウント所有」警告で 登録 自体出ないため、二重に守られる。

### Udon クライアント向け (GET のみ)

セーブデータは `internal/savedata.Data` 構造体で表現される JSON オブジェクト。
現状のフィールドは `score` (int64) と `generated_at` (RFC3339)。フィールドを追加する際は

1. `internal/savedata/savedata.go` の `Data` 構造体に **末尾追加** で field を生やす
2. `save_history` / `latest_saves` に対応するカラムを `ALTER TABLE ADD COLUMN` で追加
3. `internal/db/saves.go` の `Save` の INSERT と `scanSaveEntry` の SELECT を拡張

の3点をセットで行う。署名入力はこの JSON のキャノニカルバイト列。

#### JWT の扱い (`/save` と `/load` 共通)

`jwt` クエリは **オプション**。次のルールで動作する:

- `display_name` が **未登録** (DB に存在しない) なら JWT なしで通る。署名のみ検証
- `display_name` が **登録済み** なら JWT 必須。`display_name` が JWT クレームと一致し、
  かつ JTI が当該ユーザーの現行 JTI かつブラックリスト未登録のときだけ受理 (`401` 以外)
- JWT を付けるなら、`display_name` と JWT クレームの DisplayName は一致必須 (`401`)

これは、Udon に埋め込まれた署名鍵が抽出可能であることを前提とした多層防御:
正規ユーザーは一度登録すれば JWT で自分の DisplayName をロックでき、第三者が
署名鍵だけでは上書きできない (詳しくは「既知の制約」参照)。

#### `GET /save?data=<urlencoded JSON>&display_name=<name>&sig=<hex>[&jwt=<JWT>]`

- `data` は `savedata.Data` をシリアライズした JSON (例: `{"score":1234,"generated_at":"2026-05-11T12:00:00Z"}`)。
  URL クエリに載せる際は URL エンコードする
- `sig` = `SipHash-2-4(SAVE_SECRET, <data raw bytes> ‖ 0x00 ‖ <display_name bytes>)` → lowercase hex (16文字)
  (パートは 1 バイトの `0x00` で連結)
- `data.generated_at` はサーバ時刻に対して `-7d ～ +5min` の範囲外なら `400`
- 同一 `display_name` で同一 `generated_at` が既に保存済みなら `409`
- レスポンス: `success`

#### `GET /load?display_name=<name>&sig=<hex>[&jwt=<JWT>]`

- リクエスト sig = `SipHash-2-4(LOAD_SECRET, <display_name bytes>)` → lowercase hex (16文字)
- セーブなしの場合は `404`
- **レスポンス (200)**: JSON

```json
{"data":{"score":1234,"generated_at":"2026-05-11T12:00:00Z"},"sig":"<hex>"}
```

レスポンス `sig` = `SipHash-2-4(LOAD_SECRET, <data の raw JSON bytes>)` → lowercase hex (16文字)
(リクエスト sig とは別キーフィールド構成: リクエストは display_name のみ、レスポンスは data のみ)

Udon クライアント側でこの sig を検証することで MITM やクエリ書き換えによる改ざんを検知できる。
`data` フィールドは server がエンコードしたバイト列をそのまま受け取り (空白なし、
field 順は構造体定義順) 署名を計算する想定。

#### `GET /ranking?limit=<1..1000>&verified=<true|false>`

セーブ済み・BAN されていないユーザーの JSON 配列を返す。

- `limit` 省略時は 10、`verified` 省略時は `false`
- `verified=true` を指定すると JWT 付きで保存されたエントリだけに絞り込む
- 各エントリには `verified` フィールドが付き、そのセーブが JWT 認証下で行われたかを示す

レスポンス例:

```json
[
  {"rank":1,"display_name":"alice","score":1234,"updated_at":"2026-05-11T12:00:00Z","verified":true}
]
```

## 署名対象バイト列フォーマット

アルゴリズムは SipHash-2-4 (16 バイト鍵、64bit タグ → lowercase hex 16 文字)。

| 場面                         | メッセージ                                       | 鍵            | 方向                    |
| ---------------------------- | ------------------------------------------------ | ------------- | ----------------------- |
| `/save` リクエスト sig       | `<data raw bytes> ‖ 0x00 ‖ <display_name bytes>` | `SAVE_SECRET` | クライアント → サーバー |
| `/load` リクエスト sig       | `<display_name bytes>`                           | `LOAD_SECRET` | クライアント → サーバー |
| `/load` レスポンス sig       | レスポンス内 `data` の raw bytes                 | `LOAD_SECRET` | サーバー → クライアント |
| `/auth/start` リクエスト sig | `<display_name bytes>`                           | `AUTH_SECRET` | クライアント → サーバー |

パートが複数あるものは 1 バイトの `0x00` 区切りで連結する (実装: [internal/auth/sig.go](internal/auth/sig.go))。
`data` 部のキャノニカル表現は `savedata.Data` を Go の `encoding/json` で出力したバイト列
(空白なし、struct 定義順) を定義として用いる。

## ディレクトリ構成

```
cmd/api      — HTTP サーバ (Udon API + OAuth web flow)
cmd/admin    — 管理者 CLI (ban / whois / 等)
cmd/vrcsim   — VRChat Udon クライアント模擬 CLI
api/                  — OpenAPI 仕様 (openapi.yaml + embed)
internal/api          — HTTP ハンドラ
internal/auth         — JWT (HS256) / SipHash 署名ヘルパ
internal/clock        — Clock 抽象 (テストで時刻固定するため)
internal/config       — env 読み込み
internal/db           — SQLite スキーマ + リポジトリ
internal/idgen        — UUID 生成 (テスト差し替え可能)
internal/oauth        — Discord OAuth Provider + テスト用 Fake
internal/registration — JWT 発行コア (OAuth callback / e2e CLI 共通)
internal/savedata     — セーブ JSON のシリアライズ規約
internal/vrcclient    — Udon が叩く URL の組み立て + GET 実行
```
