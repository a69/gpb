package authz

import (
	"fmt"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

func TestRegistry(t *testing.T) {
	t.Run("register and lookup", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(Tenant{Name: "alpha", GroupChatID: "g-1"})

		got, ok := reg.ByGroup("g-1")
		if !ok {
			t.Fatal("expected tenant to be found")
		}
		if got.Name != "alpha" {
			t.Errorf("Name = %q, want alpha", got.Name)
		}

		_, ok = reg.ByGroup("g-unknown")
		if ok {
			t.Error("ByGroup for unknown should return false")
		}
	})

	t.Run("register overwrites same group", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(Tenant{Name: "first", GroupChatID: "g-1"})
		reg.Register(Tenant{Name: "second", GroupChatID: "g-1"})

		got, _ := reg.ByGroup("g-1")
		if got.Name != "second" {
			t.Errorf("Name = %q, want second (last write wins)", got.Name)
		}
	})

	t.Run("byGroup returns a copy", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(Tenant{Name: "alpha", GroupChatID: "g-1"})

		got, _ := reg.ByGroup("g-1")
		got.Name = "mutated"

		got2, _ := reg.ByGroup("g-1")
		if got2.Name != "alpha" {
			t.Error("ByGroup should return a copy; registry was mutated")
		}
	})

	t.Run("all returns snapshot", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(Tenant{Name: "a", GroupChatID: "g1"})
		reg.Register(Tenant{Name: "b", GroupChatID: "g2"})

		all := reg.All()
		if len(all) != 2 {
			t.Fatalf("All() len = %d, want 2", len(all))
		}

		// Mutating returned slice must not affect registry
		all[0].Name = "hacked"
		got, _ := reg.ByGroup("g1")
		if got.Name == "hacked" {
			t.Error("All() should return copy; registry was mutated via slice")
		}
	})

	t.Run("all empty registry", func(t *testing.T) {
		reg := NewRegistry()
		all := reg.All()
		if len(all) != 0 {
			t.Errorf("All() len = %d, want 0", len(all))
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		reg := NewRegistry()
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				reg.Register(Tenant{Name: fmt.Sprintf("t%d", i), GroupChatID: fmt.Sprintf("g%d", i)})
				reg.ByGroup(fmt.Sprintf("g%d", i))
				reg.All()
			}(i)
		}
		wg.Wait()
	})
}

func TestGroupGuard(t *testing.T) {
	t.Run("allow with matching secret", func(t *testing.T) {
		reg := NewRegistry()
		guard := NewGroupGuard(reg, "secret123")

		req := httptest.NewRequest("POST", "/webhook", nil)
		req.Header.Set("X-Bot-Token", "secret123")

		if !guard.Allow(req) {
			t.Error("Allow() should return true with matching secret")
		}
	})

	t.Run("deny with wrong secret", func(t *testing.T) {
		guard := NewGroupGuard(NewRegistry(), "secret123")

		req := httptest.NewRequest("POST", "/webhook", nil)
		req.Header.Set("X-Bot-Token", "wrong")

		if guard.Allow(req) {
			t.Error("Allow() should return false with wrong secret")
		}
	})

	t.Run("deny with missing header", func(t *testing.T) {
		guard := NewGroupGuard(NewRegistry(), "secret123")

		req := httptest.NewRequest("POST", "/webhook", nil)

		if guard.Allow(req) {
			t.Error("Allow() should return false when header missing")
		}
	})

	t.Run("allow without secret in dev mode", func(t *testing.T) {
		guard := NewGroupGuard(NewRegistry(), "")

		req := httptest.NewRequest("POST", "/webhook", nil)

		if !guard.Allow(req) {
			t.Error("Allow() should return true when no secret configured (dev mode)")
		}
	})

	t.Run("check group", func(t *testing.T) {
		reg := NewRegistry()
		reg.Register(Tenant{GroupChatID: "g-1"})
		guard := NewGroupGuard(reg, "")

		if !guard.CheckGroup("g-1") {
			t.Error("CheckGroup() should return true for registered group")
		}
		if guard.CheckGroup("g-unknown") {
			t.Error("CheckGroup() should return false for unknown group")
		}
	})
}

func TestFilterFields(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]string
		output map[string]string
	}{
		{
			name:   "empty",
			input:  map[string]string{},
			output: map[string]string{},
		},
		{
			name:   "all non-empty redacted",
			input:  map[string]string{"token": "secret123", "key": "abc"},
			output: map[string]string{"token": "***", "key": "***"},
		},
		{
			name:   "empty value stays empty",
			input:  map[string]string{"token": "", "key": "abc"},
			output: map[string]string{"token": "", "key": "***"},
		},
		{
			name:   "all empty",
			input:  map[string]string{"a": "", "b": ""},
			output: map[string]string{"a": "", "b": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterFields(tt.input)

			// Verify output matches expected
			if !reflect.DeepEqual(got, tt.output) {
				t.Errorf("FilterFields() = %v, want %v", got, tt.output)
			}

			// Verify original map was not mutated
			for k, v := range tt.input {
				if v != "" && got[k] != "***" {
					t.Errorf("original map was mutated: %s = %s", k, tt.input[k])
				}
			}
		})
	}
}
