// admin is a server-host CLI for ban/unban/whois/release-name/invalidate-token
// operations. It connects directly to the SQLite DB (so it requires
// filesystem access to data/vrc.db) and bypasses the HTTP layer entirely.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/njm2360/vrchat-ranking-system/internal/config"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	sub := os.Args[1]
	args := os.Args[2:]

	cfg, err := config.Load()
	exitIf(err)

	database, err := db.Open(cfg.DBPath)
	exitIf(err)
	defer database.Close()
	ctx := context.Background()

	switch sub {
	case "ban":
		runBan(ctx, database, args)
	case "unban":
		runUnban(ctx, database, args)
	case "whois":
		runWhois(ctx, database, args)
	case "release-name":
		runReleaseName(ctx, database, args)
	case "invalidate-token":
		runInvalidateToken(ctx, database, args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", sub)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `admin — vrcsim ranking system administrative CLI

Subcommands:
  ban --discord-id <id> [--reason <text>]
      Ban a Discord user (pre-registration ban allowed).

  ban --name <DisplayName> [--reason <text>]
      Ban a VRChat display name: hides it from ranking and blocks registration.
      Useful when the bad actor is a VRChat user (not a Discord user) and
      the name cannot be reclaimed for ~90 days due to VRChat rename cooldown.

  unban --discord-id <id>
      Lift a Discord ban.

  unban --name <DisplayName>
      Lift a display-name ban.

  whois --name <DisplayName> | --discord-id <id>
      Show registration info for a VRChat user name or Discord user.

  release-name --name <DisplayName> [--reason <text>]
      Forcibly release a hijacked DisplayName binding so the legitimate
      VRChat owner can /auth/start?action=register.

  invalidate-token --jti <jti> [--reason <text>]
      Blacklist a single JTI.`)
}

func exitIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runBan(ctx context.Context, d *db.DB, args []string) {
	fs := flag.NewFlagSet("ban", flag.ExitOnError)
	id := fs.String("discord-id", "", "Discord user ID")
	name := fs.String("name", "", "VRChat DisplayName")
	reason := fs.String("reason", "", "reason (optional)")
	_ = fs.Parse(args)
	if (*id == "") == (*name == "") {
		exitIf(fmt.Errorf("specify exactly one of --discord-id or --name"))
	}
	if *id != "" {
		exitIf(d.BanDiscordID(ctx, *id, *reason))
		fmt.Printf("banned discord: %s\n", *id)
	} else {
		exitIf(d.BanDisplayName(ctx, *name, *reason))
		fmt.Printf("banned name: %s\n", *name)
	}
}

func runUnban(ctx context.Context, d *db.DB, args []string) {
	fs := flag.NewFlagSet("unban", flag.ExitOnError)
	id := fs.String("discord-id", "", "Discord user ID")
	name := fs.String("name", "", "VRChat DisplayName")
	_ = fs.Parse(args)
	if (*id == "") == (*name == "") {
		exitIf(fmt.Errorf("specify exactly one of --discord-id or --name"))
	}
	if *id != "" {
		exitIf(d.UnbanDiscordID(ctx, *id))
		fmt.Printf("unbanned discord: %s\n", *id)
	} else {
		exitIf(d.UnbanDisplayName(ctx, *name))
		fmt.Printf("unbanned name: %s\n", *name)
	}
}

func runWhois(ctx context.Context, d *db.DB, args []string) {
	fs := flag.NewFlagSet("whois", flag.ExitOnError)
	name := fs.String("name", "", "VRChat DisplayName")
	id := fs.String("discord-id", "", "Discord user ID")
	_ = fs.Parse(args)
	if (*name == "") == (*id == "") {
		exitIf(fmt.Errorf("specify exactly one of --name or --discord-id"))
	}

	var (
		user *db.User
		err  error
	)
	if *name != "" {
		user, err = d.GetUserByDisplayName(ctx, *name)
	} else {
		user, err = d.GetUserByDiscordID(ctx, *id)
	}
	if errors.Is(err, db.ErrUserNotFound) {
		fmt.Println("not found")
		return
	}
	exitIf(err)

	banned, _ := d.IsDiscordIDBanned(ctx, user.DiscordID)
	jtiState := "none"
	if user.CurrentJTI != "" {
		blacklisted, _ := d.IsJTIBlacklisted(ctx, user.CurrentJTI)
		if blacklisted {
			jtiState = user.CurrentJTI + " (revoked)"
		} else {
			jtiState = user.CurrentJTI + " (active)"
		}
	}
	fmt.Printf("discord_id   : %s\n", user.DiscordID)
	fmt.Printf("display_name : %s\n", user.DisplayName)
	fmt.Printf("created_at   : %s\n", user.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Printf("updated_at   : %s\n", user.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Printf("current_jti  : %s\n", jtiState)
	fmt.Printf("banned       : %v\n", banned)
}

func runReleaseName(ctx context.Context, d *db.DB, args []string) {
	fs := flag.NewFlagSet("release-name", flag.ExitOnError)
	name := fs.String("name", "", "DisplayName to release")
	reason := fs.String("reason", "admin release", "reason")
	_ = fs.Parse(args)
	if *name == "" {
		exitIf(fmt.Errorf("--name required"))
	}
	prior, err := d.ReleaseDisplayName(ctx, *name, *reason)
	if errors.Is(err, db.ErrUserNotFound) {
		exitIf(fmt.Errorf("DisplayName %q is not registered", *name))
	}
	exitIf(err)
	fmt.Printf("released %s (prior holder: %s)\n", *name, prior)
}

func runInvalidateToken(ctx context.Context, d *db.DB, args []string) {
	fs := flag.NewFlagSet("invalidate-token", flag.ExitOnError)
	jti := fs.String("jti", "", "JTI to blacklist")
	reason := fs.String("reason", "admin invalidate", "reason")
	_ = fs.Parse(args)
	if *jti == "" {
		exitIf(fmt.Errorf("--jti required"))
	}
	exitIf(d.BlacklistJTI(ctx, *jti, *reason))
	fmt.Printf("invalidated jti: %s\n", *jti)
}
