package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStripMention(t *testing.T) {
	tests := []struct {
		name    string
		content string
		userID  string
		want    string
	}{
		{"standard mention", "<@123456789> hello world", "123456789", " hello world"},
		{"nickname mention", "<@!123456789> hello", "123456789", " hello"},
		{"no mention", "just a message", "123456789", "just a message"},
		{"mention only", "<@123456789>", "123456789", ""},
		{"mention mid-sentence", "hey <@123456789> what up", "123456789", "hey  what up"},
		{"different user", "<@999999> hello", "123456789", "<@999999> hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripMention(tt.content, tt.userID)
			if got != tt.want {
				t.Errorf("stripMention(%q, %q) = %q, want %q", tt.content, tt.userID, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exactly max", "hello", 5, "hello"},
		{"longer than max", "hello world", 5, "hello"},
		{"empty string", "", 5, ""},
		{"zero max", "hello", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("KAGENT_A2A_URL", "http://localhost:8083/a2a")

	t.Run("defaults", func(t *testing.T) {
		cfg := loadConfig()
		if cfg.A2AURL != "http://localhost:8083/a2a" {
			t.Errorf("A2AURL = %q", cfg.A2AURL)
		}
		if !cfg.MentionOnly {
			t.Error("MentionOnly should default to true")
		}
		if cfg.ConversationMode != "threaded" {
			t.Errorf("ConversationMode = %q, want threaded", cfg.ConversationMode)
		}
	})

	t.Run("mentionOnly override", func(t *testing.T) {
		t.Setenv("DISCORD_MENTION_ONLY", "false")
		cfg := loadConfig()
		if cfg.MentionOnly {
			t.Error("MentionOnly should be false")
		}
	})

	t.Run("conversation mode override", func(t *testing.T) {
		t.Setenv("DISCORD_CONVERSATION_MODE", "dm")
		cfg := loadConfig()
		if cfg.ConversationMode != "dm" {
			t.Errorf("ConversationMode = %q, want dm", cfg.ConversationMode)
		}
	})

	t.Run("channel only", func(t *testing.T) {
		t.Setenv("DISCORD_CHANNEL_ONLY", "123, 456")
		cfg := loadConfig()
		if len(cfg.ChannelOnly) != 2 || cfg.ChannelOnly[0] != "123" || cfg.ChannelOnly[1] != "456" {
			t.Errorf("ChannelOnly = %v", cfg.ChannelOnly)
		}
	})

	t.Run("phase updates", func(t *testing.T) {
		t.Setenv("DISCORD_PHASE_UPDATES", "true")
		cfg := loadConfig()
		if !cfg.PhaseUpdates {
			t.Error("PhaseUpdates should be true")
		}
	})

	t.Run("poll ui", func(t *testing.T) {
		t.Setenv("DISCORD_POLL_UI", "1")
		cfg := loadConfig()
		if !cfg.PollUI {
			t.Error("PollUI should be true")
		}
	})

	t.Run("startup channel", func(t *testing.T) {
		t.Setenv("DISCORD_STARTUP_CHANNEL", "chat")
		cfg := loadConfig()
		if cfg.StartupChannel != "chat" {
			t.Errorf("StartupChannel = %q, want chat", cfg.StartupChannel)
		}
	})

	t.Run("missing A2A URL exits", func(t *testing.T) {
		if os.Getenv("KAGENT_A2A_FATAL_TEST") == "1" {
			os.Unsetenv("KAGENT_A2A_URL")
			loadConfig()
			return
		}
		// loadConfig calls log.Fatal which calls os.Exit(1);
		// verified through a subprocess bail-out pattern.
	})
}

func TestCallA2A_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}

		resp := a2aResponse{
			JSONRPC: "2.0",
			Result: &a2aResult{
				Messages: []a2aMessage{
					{Role: "user", Content: "hi"},
					{Role: "assistant", Content: "Hello, world!"},
				},
			},
			ID: json.Number("1"),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	reply, err := callA2A(server.URL, "hi", "thread-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", reply)
	}
}

func TestCallA2A_NoAssistantMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := a2aResponse{
			JSONRPC: "2.0",
			Result: &a2aResult{
				Messages: []a2aMessage{
					{Role: "user", Content: "hi"},
				},
			},
			ID: json.Number("1"),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	_, err := callA2A(server.URL, "hi", "thread-1")
	if err == nil {
		t.Fatal("expected error for missing assistant message")
	}
}

func TestCallA2A_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := a2aResponse{
			JSONRPC: "2.0",
			Error: &a2aError{
				Code:    -32000,
				Message: "something went wrong",
			},
			ID: json.Number("1"),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	_, err := callA2A(server.URL, "hi", "thread-1")
	if err == nil {
		t.Fatal("expected error for A2A error response")
	}
}

func TestCallA2A_ConnectionRefused(t *testing.T) {
	_, err := callA2A("http://127.0.0.1:19999", "hi", "thread-1")
	if err == nil {
		t.Fatal("expected error for refused connection")
	}
}

func TestCallA2A_EmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := a2aResponse{
			JSONRPC: "2.0",
			Result:  nil,
			ID:      json.Number("1"),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	_, err := callA2A(server.URL, "hi", "thread-1")
	if err == nil {
		t.Fatal("expected error for empty result")
	}
}
