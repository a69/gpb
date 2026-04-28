package command

import (
	"context"
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		text string
		want Command
	}{
		{name: "simple slash command", text: "/status", want: Command{Name: "status"}},
		{name: "with bot mention", text: "/status@gpb_bot", want: Command{Name: "status"}},
		{name: "with single arg", text: "/status --full", want: Command{Name: "status", Args: []string{"--full"}}},
		{name: "with multiple args", text: "/status arg1 arg2", want: Command{Name: "status", Args: []string{"arg1", "arg2"}}},
		{name: "empty string", text: "", want: Command{}},
		{name: "only whitespace", text: "   ", want: Command{}},
		{name: "plain text no slash", text: "hello", want: Command{}},
		{name: "just slash", text: "/", want: Command{}},
		{name: "at sign in args preserved", text: "/status user@example.com", want: Command{Name: "status", Args: []string{"user@example.com"}}},
		{name: "slash with leading spaces", text: "  /status  ", want: Command{Name: "status"}},
		{name: "mention without command", text: "@bot hello", want: Command{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.text)
			if got.Name != tt.want.Name {
				t.Errorf("Parse() Name = %q, want %q", got.Name, tt.want.Name)
			}
			if len(got.Args) != len(tt.want.Args) {
				t.Errorf("Parse() Args len = %d, want %d (%v vs %v)", len(got.Args), len(tt.want.Args), got.Args, tt.want.Args)
				return
			}
			for i := range got.Args {
				if got.Args[i] != tt.want.Args[i] {
					t.Errorf("Parse() Args[%d] = %q, want %q", i, got.Args[i], tt.want.Args[i])
				}
			}
		})
	}
}

func TestRouterDispatch(t *testing.T) {
	router := NewRouter()

	t.Run("register and dispatch", func(t *testing.T) {
		router.Register("status", func(_ context.Context, cmd Command) (string, error) {
			return "report generated", nil
		})

		got, err := router.Dispatch(context.Background(), Command{Name: "status"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "report generated" {
			t.Errorf("Dispatch() = %q, want %q", got, "report generated")
		}
	})

	t.Run("unknown command returns suggestion", func(t *testing.T) {
		got, err := router.Dispatch(context.Background(), Command{Name: "foo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "Unknown command: /foo. Try /status." {
			t.Errorf("Dispatch() = %q, want suggestion", got)
		}
	})

	t.Run("handler error propagates", func(t *testing.T) {
		router.Register("fail", func(_ context.Context, cmd Command) (string, error) {
			return "", errors.New("something went wrong")
		})
		_, err := router.Dispatch(context.Background(), Command{Name: "fail"})
		if err == nil {
			t.Fatal("expected error from handler")
		}
	})

	t.Run("handler returns empty reply", func(t *testing.T) {
		router.Register("silent", func(_ context.Context, cmd Command) (string, error) {
			return "", nil
		})
		got, err := router.Dispatch(context.Background(), Command{Name: "silent"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("Dispatch() = %q, want empty", got)
		}
	})
}
