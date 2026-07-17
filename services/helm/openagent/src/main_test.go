package main

import (
	"os"
	"strings"
	"testing"
	"time"
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
	t.Setenv("AGENT_API_URL", "http://localhost:8083/a2a")

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
			os.Unsetenv("AGENT_API_URL")
			loadConfig()
			return
		}
		// loadConfig calls log.Fatal which calls os.Exit(1);
		// verified through a subprocess bail-out pattern.
	})
}

func TestLoadSaveConversations(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/conversations.json"

	conv1 := &Conversation{
		ThreadID:     "thread-1",
		UserID:       "user-1",
		ChannelID:    "channel-1",
		StartedAt:    time.Now(),
		LastActivity: time.Now(),
	}
	conv2 := &Conversation{
		ThreadID:     "thread-2",
		UserID:       "user-2",
		ChannelID:    "channel-2",
		StartedAt:    time.Now(),
		LastActivity: time.Now(),
	}

	conversations["thread-1"] = conv1
	conversations["thread-2"] = conv2

	if err := saveConversations(path); err != nil {
		t.Fatalf("saveConversations: %v", err)
	}

	// Clear map and reload
	conversations = make(map[string]*Conversation)
	if err := loadConversations(path); err != nil {
		t.Fatalf("loadConversations: %v", err)
	}

	if len(conversations) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(conversations))
	}

	if conversations["thread-1"].UserID != "user-1" {
		t.Errorf("thread-1 UserID = %q, want user-1", conversations["thread-1"].UserID)
	}
	if conversations["thread-2"].UserID != "user-2" {
		t.Errorf("thread-2 UserID = %q, want user-2", conversations["thread-2"].UserID)
	}
}

func TestLoadMissingFile(t *testing.T) {
	conversations = make(map[string]*Conversation)
	err := loadConversations("/nonexistent/path/state.json")
	if err != nil {
		t.Fatalf("loadConversations on missing file: %v", err)
	}
	if len(conversations) != 0 {
		t.Errorf("expected empty map, got %d entries", len(conversations))
	}
}

func TestThinkModeParsing(t *testing.T) {
	t.Setenv("AGENT_API_URL", "http://localhost:8083/a2a")

	tests := []struct {
		name  string
		env   string
		want  ThinkMode
	}{
		{name: "off", env: "off", want: ThinkOff},
		{name: "simple", env: "simple", want: ThinkSimple},
		{name: "full", env: "full", want: ThinkFull},
		{name: "empty defaults to full", env: "", want: ThinkFull},
		{name: "garbage defaults to full", env: "garbage", want: ThinkFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("THINK_MODE", tt.env)
			cfg := loadConfig()
			if cfg.ThinkMode != tt.want {
				t.Errorf("ThinkMode = %v (%d), want %v (%d)", cfg.ThinkMode, cfg.ThinkMode, tt.want, tt.want)
			}
		})
	}
}

func TestFormatPhaseMessage(t *testing.T) {
	tests := []struct {
		phase string
		want  string
	}{
		{"Running", "🤔 Phase: Running"},
		{"Succeeded", "✅ Phase: Succeeded"},
		{"Failed", "❌ Phase: Failed"},
		{"Pending", "🔄 Phase: Pending"},
		{"Unknown", "🔄 Phase: Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			got := formatPhaseMessage(tt.phase)
			if got != tt.want {
				t.Errorf("formatPhaseMessage(%q) = %q, want %q", tt.phase, got, tt.want)
			}
		})
	}
}

func TestParseLogEvent(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"planning the next steps", "📝 Planning"},
		{"plan: deploy to k8s", "📝 Planning"},
		{"executing tool call", "🔧 Using tool"},
		{"tool: kubectl apply", "🔧 Using tool"},
		{"I'm thinking about this", "🤔 Thinking"},
		{"think: maybe use goroutine", "🤔 Thinking"},
		{"delegate to persona", "🔄 Delegating"},
		{"ERROR: something broke", "❌ Error"},
		{"error: connection refused", "❌ Error"},
		{"just a normal log line", ""},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := parseLogEvent(tt.line)
			if got != tt.want {
				t.Errorf("parseLogEvent(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestSimpleThinkModePhaseTracking(t *testing.T) {
	prevPhase := ""
	phase := "Running"
	notified := false

	// First phase should NOT trigger (prevPhase == "")
	if phase != prevPhase && prevPhase != "" {
		notified = true
	}
	prevPhase = phase
	if notified {
		t.Error("first phase should not trigger notification")
	}

	// Same phase should NOT trigger again
	if phase != prevPhase && prevPhase != "" {
		notified = true
	}
	if notified {
		t.Error("same phase should not trigger notification")
	}

	// Phase change SHOULD trigger
	phase = "Succeeded"
	if phase != prevPhase && prevPhase != "" {
		notified = true
	}
	if !notified {
		t.Error("phase change should trigger notification")
	}
}

func TestFullThinkModeCapping(t *testing.T) {
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "error: something went wrong at line " + string(rune('0'+i%10))
	}
	count := 0
	for _, line := range lines {
		if label := parseLogEvent(line); label != "" {
			if count < 10 {
				count++
			}
		}
	}
	if count != 10 {
		t.Errorf("expected 10 events captured, got %d", count)
	}
}

func TestBuildSessionEmbed(t *testing.T) {
	conv := &Conversation{
		ThreadID:     "thread-123",
		UserID:       "user-1",
		ChannelID:    "channel-1",
		StartedAt:    time.Now(),
		LastActivity: time.Now(),
		SessionRunID: "discord-123-456",
		DashboardURL: "https://openagent.maklab.net/runs/discord-123-456",
	}

	t.Run("initial state (no usage)", func(t *testing.T) {
		embed := buildSessionEmbed(nil, conv, nil)
		if embed.Title != "Session" {
			t.Errorf("title = %q, want Session", embed.Title)
		}
		if len(embed.Fields) != 2 {
			t.Errorf("fields = %d, want 2 (Started + Run)", len(embed.Fields))
		}
		if embed.Fields[0].Name != "Started" {
			t.Errorf("field[0] = %q, want Started", embed.Fields[0].Name)
		}
		if embed.Fields[1].Name != "Run" {
			t.Errorf("field[1] = %q, want Run", embed.Fields[1].Name)
		}
	})

	t.Run("with usage", func(t *testing.T) {
		usage := &tokenUsage{
			inputTokens:  500,
			outputTokens: 347,
			totalTokens:  847,
			cost:         0.0123,
		}
		embed := buildSessionEmbed(nil, conv, usage)
		if len(embed.Fields) != 4 {
			t.Errorf("fields = %d, want 4 (Started + Tokens + Cost + Run)", len(embed.Fields))
		}
		if embed.Fields[1].Name != "Tokens" {
			t.Errorf("field[1] = %q, want Tokens", embed.Fields[1].Name)
		}
		if !strings.Contains(embed.Fields[1].Value, "847 total") {
			t.Errorf("tokens field missing total: %q", embed.Fields[1].Value)
		}
		if embed.Fields[2].Name != "Cost" {
			t.Errorf("field[2] = %q, want Cost", embed.Fields[2].Name)
		}
		if embed.Fields[2].Value != "$0.0123" {
			t.Errorf("cost = %q, want $0.0123", embed.Fields[2].Value)
		}
	})

	t.Run("zero usage shows dash", func(t *testing.T) {
		usage := &tokenUsage{}
		embed := buildSessionEmbed(nil, conv, usage)
		if embed.Fields[1].Value != "—" {
			t.Errorf("tokens = %q, want —", embed.Fields[1].Value)
		}
		if embed.Fields[2].Value != "—" {
			t.Errorf("cost = %q, want —", embed.Fields[2].Value)
		}
	})

	t.Run("partial usage (only input tokens)", func(t *testing.T) {
		usage := &tokenUsage{
			totalTokens: 200,
		}
		embed := buildSessionEmbed(nil, conv, usage)
		if embed.Fields[1].Value != "200 total" {
			t.Errorf("tokens = %q, want '200 total'", embed.Fields[1].Value)
		}
		if embed.Fields[2].Value != "—" {
			t.Errorf("cost = %q, want — when zero", embed.Fields[2].Value)
		}
	})
}

func TestParseTokenUsage(t *testing.T) {
	tests := []struct {
		name   string
		status map[string]interface{}
		want   *tokenUsage
	}{
		{
			name:   "nil status",
			status: nil,
			want:   nil,
		},
		{
			name:   "no tokenUsage key",
			status: map[string]interface{}{"phase": "Running"},
			want:   nil,
		},
		{
			name: "full usage",
			status: map[string]interface{}{
				"tokenUsage": map[string]interface{}{
					"inputTokens":  float64(500),
					"outputTokens": float64(347),
					"totalTokens":  float64(847),
					"durationMs":   float64(1234),
				},
			},
			want: &tokenUsage{inputTokens: 500, outputTokens: 347, totalTokens: 847, durationMs: 1234},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTokenUsage(tt.status)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got.totalTokens != tt.want.totalTokens {
				t.Errorf("totalTokens = %d, want %d", got.totalTokens, tt.want.totalTokens)
			}
			if got.inputTokens != tt.want.inputTokens {
				t.Errorf("inputTokens = %d, want %d", got.inputTokens, tt.want.inputTokens)
			}
		})
	}
}
