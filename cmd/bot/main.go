package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/config"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

type bot struct {
	cfg *config.Config
	db  *db.DB
	reg *registration.Service
	log *slog.Logger
}

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "register",
		Description: "発行したチケットをトークンに引き換えます (再実行で再発行)",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "uuid", Description: "VRChatワールド内で取得したチケットUUID", Required: true},
		},
	},
	{
		Name:        "mytoken",
		Description: "現在のトークンを再表示します",
	},
	{
		Name:        "unregister",
		Description: "ランキングから自分を除外します",
	},
	{
		Name:        "whois",
		Description: "(管理者用) ユーザー名またはDiscordユーザーから登録情報を引きます",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "VRChatのユーザー名"},
			{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Discordユーザー"},
		},
	},
	{
		Name:        "release-name",
		Description: "(管理者用) 不正に取得されたVRChatユーザー名を解放します",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "解放するVRChatユーザー名", Required: true},
			{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "理由 (任意)"},
		},
	},
	{
		Name:        "ban",
		Description: "(管理者用) 指定ユーザーをランキングから除外します",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "BAN対象のユーザー (メンションまたはID)", Required: true},
			{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "理由 (任意)"},
		},
	},
	{
		Name:        "unban",
		Description: "(管理者用) BANを解除します",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "BAN解除対象のユーザー (メンションまたはID)", Required: true},
		},
	},
	{
		Name:        "invalidate-token",
		Description: "(管理者用) 特定のトークンを無効化します",
		Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "jti", Description: "無効化したいJWTのjti", Required: true},
			{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "理由 (任意)"},
		},
	},
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}
	if cfg.BotToken == "" {
		log.Error("BOT_TOKEN is empty — set it in .env to run the bot")
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	b := &bot{
		cfg: cfg,
		db:  database,
		reg: registration.New(database, auth.NewJWTIssuer(cfg.JWTSecret)),
		log: log,
	}

	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Error("discordgo new", "err", err)
		os.Exit(1)
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	dg.AddHandler(b.onInteraction)

	if err := dg.Open(); err != nil {
		log.Error("discord open", "err", err)
		os.Exit(1)
	}
	defer dg.Close()

	guildID := cfg.BotGuildID
	registered := make([]*discordgo.ApplicationCommand, 0, len(commands))
	for _, c := range commands {
		cmd, err := dg.ApplicationCommandCreate(dg.State.User.ID, guildID, c)
		if err != nil {
			log.Error("register command", "name", c.Name, "err", err)
			continue
		}
		registered = append(registered, cmd)
	}
	log.Info("bot ready", "commands", len(registered), "guild", guildID)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
}

func (b *bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	data := i.ApplicationCommandData()
	switch data.Name {
	case "register":
		b.handleRegister(s, i, data)
	case "mytoken":
		b.handleMyToken(s, i)
	case "unregister":
		b.handleUnregister(s, i)
	case "whois":
		b.handleWhois(s, i, data)
	case "release-name":
		b.handleReleaseName(s, i, data)
	case "ban":
		b.handleBan(s, i, data)
	case "unban":
		b.handleUnban(s, i, data)
	case "invalidate-token":
		b.handleInvalidate(s, i, data)
	}
}

func optString(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range opts {
		if o.Name == name {
			return o.StringValue()
		}
	}
	return ""
}

// optUserID extracts a Discord user ID from a User-type option. Works whether
// the admin entered a mention (@name) or pasted a raw user ID — Discord
// resolves both to the same User object before it reaches us.
func optUserID(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range opts {
		if o.Name == name {
			if u := o.UserValue(nil); u != nil {
				return u.ID
			}
		}
	}
	return ""
}

func ephemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func userID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// registrationErrorMessage maps a registration error to a user-friendly
// Japanese message.
func registrationErrorMessage(err error) string {
	switch {
	case errors.Is(err, registration.ErrBanned):
		return "❌ このアカウントはBANされているため登録できません。"
	case errors.Is(err, registration.ErrTicketNotFound):
		return "❌ チケットが見つかりませんでした。チケットを再発行してください。"
	case errors.Is(err, registration.ErrTicketExpired):
		return "❌ チケットの有効期限が切れています。チケットを再発行してください。"
	case errors.Is(err, registration.ErrTicketUsed):
		return "❌ このチケットは既に使用済みです。チケットを再発行してください。"
	case errors.Is(err, registration.ErrDisplayNameTaken):
		return "❌ このユーザー名は既に他のDiscordアカウントで登録されています。\n" +
			"心当たりがない場合は管理者にお問い合わせください。"
	}
	return fmt.Sprintf("❌ 登録に失敗しました: %v", err)
}

func (b *bot) handleRegister(s *discordgo.Session, i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	uuid := optString(data.Options, "uuid")
	uid := userID(i)
	if uid == "" {
		ephemeral(s, i, "❌ Discord IDを取得できませんでした。")
		return
	}

	res, err := b.reg.Register(context.Background(), uid, uuid)
	if err != nil {
		ephemeral(s, i, registrationErrorMessage(err))
		return
	}

	var body string
	switch {
	case res.IsRenewal && res.PrevDisplayName != res.DisplayName:
		body = fmt.Sprintf("🔄 トークンを再発行しました (旧トークンは無効化済みです)\nユーザー名: `%s` → `%s`\n\n以下のトークンをワールド内のテキストボックスに貼り付けてください:\n```\n%s\n```",
			res.PrevDisplayName, res.DisplayName, res.JWT)
	case res.IsRenewal:
		body = fmt.Sprintf("🔄 トークンを再発行しました (旧トークンは無効化済みです)\nユーザー名: `%s`\n\n以下のトークンをワールド内のテキストボックスに貼り付けてください:\n```\n%s\n```",
			res.DisplayName, res.JWT)
	default:
		body = fmt.Sprintf("✅ 登録しました\nユーザー名: `%s`\n\n以下のトークンをワールド内のテキストボックスに貼り付けてください:\n```\n%s\n```",
			res.DisplayName, res.JWT)
	}
	ephemeral(s, i, body)
}

func (b *bot) handleMyToken(s *discordgo.Session, i *discordgo.InteractionCreate) {
	uid := userID(i)
	if uid == "" {
		ephemeral(s, i, "❌ Discord IDを取得できませんでした。")
		return
	}
	jwt, dn, err := b.db.GetCurrentJWT(context.Background(), uid)
	if err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			ephemeral(s, i, "❌ トークンが登録されていません。先に `/register` を実行してください。")
			return
		}
		b.log.Error("get current jwt", "err", err)
		ephemeral(s, i, "❌ サーバーエラーが発生しました。時間をおいて再度お試しください。")
		return
	}
	ephemeral(s, i, fmt.Sprintf("ユーザー名: `%s`\n\nあなたのトークン:\n```\n%s\n```", dn, jwt))
}

func (b *bot) handleUnregister(s *discordgo.Session, i *discordgo.InteractionCreate) {
	uid := userID(i)
	if uid == "" {
		ephemeral(s, i, "❌ Discord IDを取得できませんでした。")
		return
	}
	if err := b.db.Unregister(context.Background(), uid); err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			ephemeral(s, i, "❌ 登録されていません。")
			return
		}
		b.log.Error("unregister", "err", err)
		ephemeral(s, i, "❌ 登録解除に失敗しました。")
		return
	}
	ephemeral(s, i, "✅ ランキングから除外しました。\n"+
		"VRChatユーザー名の予約は維持されるため、他人に名前を奪われることはありません。\n"+
		"再度参加したい場合は `/register` で新しいトークンを発行できます。")
}

func (b *bot) handleWhois(s *discordgo.Session, i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if !b.cfg.IsAdmin(userID(i)) {
		ephemeral(s, i, "❌ このコマンドは管理者専用です。")
		return
	}
	name := optString(data.Options, "name")
	uid := optUserID(data.Options, "user")
	if name == "" && uid == "" {
		ephemeral(s, i, "❌ `name` または `user` のいずれかを指定してください。")
		return
	}
	if name != "" && uid != "" {
		ephemeral(s, i, "❌ `name` と `user` は同時に指定できません。")
		return
	}

	ctx := context.Background()
	var user *db.User
	var err error
	if name != "" {
		user, err = b.db.GetUserByDisplayName(ctx, name)
	} else {
		user, err = b.db.GetUserByDiscordID(ctx, uid)
	}
	if err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			ephemeral(s, i, "🔍 該当する登録は見つかりませんでした。")
			return
		}
		b.log.Error("whois lookup", "err", err)
		ephemeral(s, i, "❌ 検索に失敗しました。")
		return
	}

	banned, err := b.db.IsBanned(ctx, user.DiscordID)
	if err != nil {
		b.log.Error("whois ban check", "err", err)
		banned = false
	}
	jtiActive := "なし"
	if user.CurrentJTI != "" {
		blacklisted, err := b.db.IsJTIBlacklisted(ctx, user.CurrentJTI)
		if err != nil {
			b.log.Error("whois jti check", "err", err)
		}
		if blacklisted {
			jtiActive = fmt.Sprintf("`%s` (無効化済み — `/unregister` または管理操作によるもの)", user.CurrentJTI)
		} else {
			jtiActive = fmt.Sprintf("`%s` (有効)", user.CurrentJTI)
		}
	}
	banLine := "なし"
	if banned {
		banLine = "**BAN中**"
	}

	ephemeral(s, i, fmt.Sprintf(
		"🔍 登録情報\n"+
			"- Discord: <@%s> (`%s`)\n"+
			"- VRChatユーザー名: `%s`\n"+
			"- 登録日時: <t:%d:f>\n"+
			"- 最終更新: <t:%d:f>\n"+
			"- 現在のJTI: %s\n"+
			"- BAN: %s",
		user.DiscordID, user.DiscordID,
		user.DisplayName,
		user.CreatedAt.Unix(),
		user.UpdatedAt.Unix(),
		jtiActive,
		banLine,
	))
}

func (b *bot) handleReleaseName(s *discordgo.Session, i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if !b.cfg.IsAdmin(userID(i)) {
		ephemeral(s, i, "❌ このコマンドは管理者専用です。")
		return
	}
	name := optString(data.Options, "name")
	if name == "" {
		ephemeral(s, i, "❌ `name` を指定してください。")
		return
	}
	reason := optString(data.Options, "reason")
	if reason == "" {
		reason = "admin release"
	}
	prior, err := b.db.ReleaseDisplayName(context.Background(), name, reason)
	if err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			ephemeral(s, i, "❌ そのユーザー名は登録されていません。")
			return
		}
		b.log.Error("release name", "err", err)
		ephemeral(s, i, "❌ 解放に失敗しました。")
		return
	}
	ephemeral(s, i, fmt.Sprintf(
		"✅ ユーザー名 `%s` を解放しました。\n"+
			"- 旧保有者: <@%s> (`%s`)\n"+
			"- 旧トークンは無効化されました",
		name, prior, prior))
}

func (b *bot) handleBan(s *discordgo.Session, i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if !b.cfg.IsAdmin(userID(i)) {
		ephemeral(s, i, "❌ このコマンドは管理者専用です。")
		return
	}
	id := optUserID(data.Options, "user")
	if id == "" {
		ephemeral(s, i, "❌ ユーザーを取得できませんでした。")
		return
	}
	reason := optString(data.Options, "reason")
	if err := b.db.Ban(context.Background(), id, reason); err != nil {
		b.log.Error("ban", "err", err)
		ephemeral(s, i, "❌ BAN登録に失敗しました。")
		return
	}
	ephemeral(s, i, fmt.Sprintf("✅ BANしました: <@%s>", id))
}

func (b *bot) handleUnban(s *discordgo.Session, i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if !b.cfg.IsAdmin(userID(i)) {
		ephemeral(s, i, "❌ このコマンドは管理者専用です。")
		return
	}
	id := optUserID(data.Options, "user")
	if id == "" {
		ephemeral(s, i, "❌ ユーザーを取得できませんでした。")
		return
	}
	if err := b.db.Unban(context.Background(), id); err != nil {
		b.log.Error("unban", "err", err)
		ephemeral(s, i, "❌ BAN解除に失敗しました。")
		return
	}
	ephemeral(s, i, fmt.Sprintf("✅ BAN解除しました: <@%s> (`%s`)", id, id))
}

func (b *bot) handleInvalidate(s *discordgo.Session, i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) {
	if !b.cfg.IsAdmin(userID(i)) {
		ephemeral(s, i, "❌ このコマンドは管理者専用です。")
		return
	}
	jti := optString(data.Options, "jti")
	reason := optString(data.Options, "reason")
	if err := b.db.BlacklistJTI(context.Background(), jti, reason); err != nil {
		b.log.Error("blacklist jti", "err", err)
		ephemeral(s, i, "❌ JTIの無効化に失敗しました。")
		return
	}
	ephemeral(s, i, fmt.Sprintf("✅ JTIを無効化しました: `%s`", jti))
}
