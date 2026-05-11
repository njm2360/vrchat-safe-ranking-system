// vrcsim mimics what a VRChat Udon client would do for E2E testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/config"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
	"github.com/njm2360/vrchat-ranking-system/internal/vrcclient"
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

	client := vrcclient.New(cfg.BaseURL, cfg.SaveSecret, cfg.LoadSecret, cfg.AuthSecret)
	ctx := context.Background()

	switch sub {
	case "save":
		runSave(ctx, client, args)
	case "load":
		runLoad(ctx, client, args)
	case "auth-start-url":
		runAuthStartURL(client, args)
	case "e2e":
		runE2E(ctx, cfg, client, args)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", sub)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `vrcsim — VRChat Udon client simulator

Subcommands:
  save --score <int> --jwt <JWT> --display-name <name> [--generated-at <RFC3339>] [--print-url]
      Sign and send /save. --generated-at defaults to current time.

  load --jwt <JWT> --display-name <name> [--print-url]
      Send /load. Prints the score (empty for no save).

  auth-start-url --display-name <name> [--fake-discord-id <id>] [--fake-username <username>]
      Print a signed /auth/start URL. Open it in a browser to begin the
      Discord OAuth flow. fake-* are only meaningful when the API server is
      running with OAUTH_MODE=mock.

  e2e --name <DisplayName> [--discord-id <id>] [--score <int>]
      Full happy-path flow with no Discord OAuth round-trip:
        register (DB direct via registration.Service) → save → load → ranking.`)
}

func exitIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runSave(ctx context.Context, client *vrcclient.Client, args []string) {
	fs := flag.NewFlagSet("save", flag.ExitOnError)
	score := fs.Int64("score", 0, "score (int)")
	jwt := fs.String("jwt", "", "JWT. Prefix with @ to read from file.")
	displayName := fs.String("display-name", "", "VRChat display name (must match JWT claim)")
	generatedAt := fs.String("generated-at", time.Now().UTC().Format(time.RFC3339), "generated_at (RFC3339、省略で現在時刻)")
	printURL := fs.Bool("print-url", false, "print URL only, do not request")
	_ = fs.Parse(args)
	if *jwt == "" {
		exitIf(fmt.Errorf("--jwt required"))
	}
	if *displayName == "" {
		exitIf(fmt.Errorf("--display-name required"))
	}
	genAt, err := time.Parse(time.RFC3339, *generatedAt)
	exitIf(err)
	jwtStr, err := readMaybeFile(*jwt)
	exitIf(err)
	p := vrcclient.SaveParams{Data: &savedata.Data{Score: *score, GeneratedAt: genAt}, JWT: jwtStr, DisplayName: *displayName}
	if *printURL {
		u, err := client.SaveURL(p)
		exitIf(err)
		fmt.Println(u)
		return
	}
	body, err := client.Save(ctx, p)
	exitIf(err)
	fmt.Println(body)
}

func runLoad(ctx context.Context, client *vrcclient.Client, args []string) {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	jwt := fs.String("jwt", "", "JWT. Prefix with @ to read from file.")
	displayName := fs.String("display-name", "", "VRChat display name (must match JWT claim)")
	printURL := fs.Bool("print-url", false, "print URL only, do not request")
	_ = fs.Parse(args)
	if *jwt == "" {
		exitIf(fmt.Errorf("--jwt required"))
	}
	if *displayName == "" {
		exitIf(fmt.Errorf("--display-name required"))
	}
	jwtStr, err := readMaybeFile(*jwt)
	exitIf(err)
	p := vrcclient.LoadParams{JWT: jwtStr, DisplayName: *displayName}
	if *printURL {
		fmt.Println(client.LoadURL(p))
		return
	}
	v, err := client.Load(ctx, p)
	exitIf(err)
	if v == nil {
		fmt.Fprintln(os.Stderr, "(no save)")
		return
	}
	fmt.Printf("score = %d\n", v.Score)
}

func runAuthStartURL(client *vrcclient.Client, args []string) {
	fs := flag.NewFlagSet("auth-start-url", flag.ExitOnError)
	displayName := fs.String("display-name", "", "VRChat display name to register")
	fakeDiscordID := fs.String("fake-discord-id", "", "(mock OAuth) Discord snowflake")
	fakeUsername := fs.String("fake-username", "", "(mock OAuth) Discord username")
	_ = fs.Parse(args)
	if *displayName == "" {
		exitIf(fmt.Errorf("--display-name required"))
	}
	fmt.Println(client.AuthStartURL(vrcclient.AuthStartParams{
		DisplayName:   *displayName,
		FakeDiscordID: *fakeDiscordID,
		FakeUsername:  *fakeUsername,
	}))
}

func runE2E(ctx context.Context, cfg *config.Config, client *vrcclient.Client, args []string) {
	fs := flag.NewFlagSet("e2e", flag.ExitOnError)
	name := fs.String("name", "", "DisplayName")
	discordID := fs.String("discord-id", "e2e-test-user", "Discord user ID (デフォルト e2e-test-user)")
	score := fs.Int64("score", 1234, "score to save")
	_ = fs.Parse(args)
	if *name == "" {
		exitIf(fmt.Errorf("--name required"))
	}

	database, err := db.Open(cfg.DBPath)
	exitIf(err)
	defer database.Close()

	fmt.Println("=> register (DB direct, bypassing Discord OAuth)")
	svc := registration.New(database, auth.NewJWTIssuer(cfg.JWTSecret))
	res, err := svc.Register(ctx, *discordID, *name)
	exitIf(err)
	if res.IsRenewal {
		fmt.Println("   (renewal — old jti blacklisted)")
	}
	fmt.Println("   JWT:", res.JWT)

	fmt.Println("=> save")
	body, err := client.Save(ctx, vrcclient.SaveParams{Data: &savedata.Data{Score: *score, GeneratedAt: time.Now().UTC()}, JWT: res.JWT, DisplayName: *name})
	exitIf(err)
	fmt.Println("   ", body)

	fmt.Println("=> load")
	loaded, err := client.Load(ctx, vrcclient.LoadParams{JWT: res.JWT, DisplayName: *name})
	exitIf(err)
	if loaded == nil {
		fmt.Println("   (no save)")
	} else {
		fmt.Println("   score =", loaded.Score)
	}

	fmt.Println("=> ranking (top 10)")
	rows, err := database.Ranking(ctx, 10, false)
	exitIf(err)
	for _, r := range rows {
		fmt.Printf("   #%d %s : %d\n", r.Rank, r.DisplayName, r.Score)
	}
}

// readMaybeFile returns the value as-is, or reads from a file if it begins with '@'.
func readMaybeFile(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if strings.HasPrefix(v, "@") {
		b, err := os.ReadFile(v[1:])
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	return v, nil
}
