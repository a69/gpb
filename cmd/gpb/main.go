package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/a69/gpb/internal/github"
	"github.com/a69/gpb/internal/msg"
	"github.com/a69/gpb/internal/reporter"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: gpb <report|notify> [flags]\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "report":
		runReport()
	case "notify":
		runNotify()
	case "poll":
		runPoll()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// resolveMessenger returns the appropriate Messenger based on flags.
// When baleToken is set, it takes precedence and implies platform=bale (backward compat).
func resolveMessenger(baleToken, platform, token string) (msg.Messenger, error) {
	if baleToken != "" {
		return msg.NewBale(baleToken), nil
	}
	if platform == "" {
		platform = "bale"
	}
	switch platform {
	case "bale":
		return msg.NewBale(token), nil
	case "telegram":
		return msg.NewTelegram(token), nil
	case "slack":
		return msg.NewSlack(token), nil
	default:
		return nil, fmt.Errorf("unknown platform: %s (valid: bale, telegram, slack)", platform)
	}
}

func urgencyDefault() int {
	if v := os.Getenv("URGENCY_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 2
}

func runReport() {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	githubToken := fs.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT")
	projectID := fs.String("project-id", os.Getenv("PROJECT_ID"), "GitHub ProjectsV2 node ID")
	baleToken := fs.String("bale-token", os.Getenv("BALE_TOKEN"), "Bale bot token (backward compat)")
	chatID := fs.String("chat-id", os.Getenv("CHAT_ID"), "Chat or channel ID")
	platform := fs.String("platform", os.Getenv("PLATFORM"), "Messaging platform: bale, telegram, slack")
	token := fs.String("token", os.Getenv("TOKEN"), "Bot token or webhook URL")
	urgencyDays := fs.Int("urgency-days", urgencyDefault(), "Days threshold for urgent flag")
	fs.Parse(os.Args[2:])

	messenger, err := resolveMessenger(*baleToken, *platform, *token)
	if err != nil {
		slog.Error("failed to create messenger", "err", err)
		os.Exit(1)
	}

	gh := github.NewClient(*githubToken)
	r := reporter.New(gh, messenger, *urgencyDays)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	slog.Info("starting daily report", "project", *projectID, "chat", *chatID)
	if err := r.SendReport(ctx, *chatID, *projectID); err != nil {
		slog.Error("report failed", "err", err)
		os.Exit(1)
	}
	slog.Info("report sent")
}

func runNotify() {
	fs := flag.NewFlagSet("notify", flag.ExitOnError)
	githubToken := fs.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT")
	itemID := fs.String("item-id", os.Getenv("ITEM_ID"), "GitHub ProjectsV2 item node ID")
	event := fs.String("event", os.Getenv("EVENT"), "Event type (created|edited|moved|deleted)")
	sender := fs.String("sender", os.Getenv("SENDER"), "GitHub username who triggered the event")
	baleToken := fs.String("bale-token", os.Getenv("BALE_TOKEN"), "Bale bot token (backward compat)")
	chatID := fs.String("chat-id", os.Getenv("CHAT_ID"), "Chat or channel ID")
	platform := fs.String("platform", os.Getenv("PLATFORM"), "Messaging platform: bale, telegram, slack")
	token := fs.String("token", os.Getenv("TOKEN"), "Bot token or webhook URL")
	urgencyDays := fs.Int("urgency-days", urgencyDefault(), "Days threshold for urgent flag")
	fs.Parse(os.Args[2:])

	messenger, err := resolveMessenger(*baleToken, *platform, *token)
	if err != nil {
		slog.Error("failed to create messenger", "err", err)
		os.Exit(1)
	}

	gh := github.NewClient(*githubToken)
	r := reporter.New(gh, messenger, *urgencyDays)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Info("sending notification", "item", *itemID, "event", *event, "sender", *sender)
	if err := r.SendNotification(ctx, *chatID, *itemID, *event, *sender); err != nil {
		slog.Error("notification failed", "err", err)
		os.Exit(1)
	}
	slog.Info("notification sent")
}

func runPoll() {
	fs := flag.NewFlagSet("poll", flag.ExitOnError)
	githubToken := fs.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT")
	projectID := fs.String("project-id", os.Getenv("PROJECT_ID"), "GitHub ProjectsV2 node ID")
	baleToken := fs.String("bale-token", os.Getenv("BALE_TOKEN"), "Bale bot token (backward compat)")
	chatID := fs.String("chat-id", os.Getenv("CHAT_ID"), "Chat or channel ID")
	stateFile := fs.String("state-file", ".gpb-state.json", "Path to state cache file")
	platform := fs.String("platform", os.Getenv("PLATFORM"), "Messaging platform: bale, telegram, slack")
	token := fs.String("token", os.Getenv("TOKEN"), "Bot token or webhook URL")
	urgencyDays := fs.Int("urgency-days", urgencyDefault(), "Days threshold for urgent flag")
	fs.Parse(os.Args[2:])

	messenger, err := resolveMessenger(*baleToken, *platform, *token)
	if err != nil {
		slog.Error("failed to create messenger", "err", err)
		os.Exit(1)
	}

	gh := github.NewClient(*githubToken)
	r := reporter.New(gh, messenger, *urgencyDays)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	slog.Info("polling project", "project", *projectID, "state", *stateFile)

	prev, err := reporter.LoadState(*stateFile)
	if err != nil {
		slog.Error("failed to load state", "err", err)
		os.Exit(1)
	}

	changes, next, err := r.Poll(ctx, *projectID, prev)
	if err != nil {
		slog.Error("poll failed", "err", err)
		os.Exit(1)
	}

	slog.Info("detected changes", "count", len(changes))

	for _, ch := range changes {
		item := ch.Item
		msgText := reporter.FormatNotification(&item, ch.Event, ch.Sender)
		if err := messenger.SendMessage(ctx, *chatID, msgText); err != nil {
			slog.Error("failed to send notification", "item", item.ID, "err", err)
			continue
		}
		slog.Info("notification sent", "item", item.ID, "event", ch.Event)
	}

	if err := reporter.SaveState(*stateFile, next); err != nil {
		slog.Error("failed to save state", "err", err)
		os.Exit(1)
	}
	slog.Info("poll complete", "items", len(next.Items))
}
