package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/a69/gpb/internal/bale"
	"github.com/a69/gpb/internal/github"
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

func runReport() {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	githubToken := fs.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT")
	projectID := fs.String("project-id", os.Getenv("PROJECT_ID"), "GitHub ProjectsV2 node ID")
	baleToken := fs.String("bale-token", os.Getenv("BALE_TOKEN"), "Bale bot token")
	chatID := fs.String("chat-id", os.Getenv("CHAT_ID"), "Bale group chat ID")
	urgencyDefault := 2
	if v := os.Getenv("URGENCY_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			urgencyDefault = n
		}
	}
	urgencyDays := fs.Int("urgency-days", urgencyDefault, "Days threshold for urgent flag")
	fs.Parse(os.Args[2:])

	gh := github.NewClient(*githubToken)
	bl := bale.NewClient(*baleToken)
	r := reporter.New(gh, bl, *urgencyDays)

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
	baleToken := fs.String("bale-token", os.Getenv("BALE_TOKEN"), "Bale bot token")
	chatID := fs.String("chat-id", os.Getenv("CHAT_ID"), "Bale group chat ID")
	urgencyDefault := 2
	if v := os.Getenv("URGENCY_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			urgencyDefault = n
		}
	}
	urgencyDays := fs.Int("urgency-days", urgencyDefault, "Days threshold for urgent flag")
	fs.Parse(os.Args[2:])

	gh := github.NewClient(*githubToken)
	bl := bale.NewClient(*baleToken)
	r := reporter.New(gh, bl, *urgencyDays)

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
	baleToken := fs.String("bale-token", os.Getenv("BALE_TOKEN"), "Bale bot token")
	chatID := fs.String("chat-id", os.Getenv("CHAT_ID"), "Bale group chat ID")
	stateFile := fs.String("state-file", ".gpb-state.json", "Path to state cache file")
	urgencyDefault := 2
	if v := os.Getenv("URGENCY_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			urgencyDefault = n
		}
	}
	urgencyDays := fs.Int("urgency-days", urgencyDefault, "Days threshold for urgent flag")
	fs.Parse(os.Args[2:])

	gh := github.NewClient(*githubToken)
	bl := bale.NewClient(*baleToken)
	r := reporter.New(gh, bl, *urgencyDays)

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
		msg := reporter.FormatNotification(&item, ch.Event, ch.Sender)
		if err := bl.SendMessage(ctx, *chatID, msg); err != nil {
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
