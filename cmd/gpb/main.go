package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
	urgencyDays := fs.Int("urgency-days", 2, "Days threshold for urgent flag")
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
	urgencyDays := fs.Int("urgency-days", 2, "Days threshold for urgent flag")
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
