package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type config struct {
	BotToken         string
	A2AURL           string
	ClientID         string
	MentionOnly      bool
	ChannelOnly      []string
	ConversationMode string // "threaded" or "dm"
	PhaseUpdates     bool
	PollUI           bool
	StartupChannel   string // Discord channel ID to send startup announcement
}

type a2aRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  a2aParams   `json:"params"`
	ID      json.Number `json:"id"`
}

type a2aParams struct {
	Messages []a2aMessage `json:"messages"`
}

type a2aMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type a2aResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  *a2aResult  `json:"result,omitempty"`
	Error   *a2aError   `json:"error,omitempty"`
	ID      json.Number `json:"id"`
}

type a2aResult struct {
	Messages []a2aMessage `json:"messages"`
}

type a2aError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Conversation tracks a single conversation thread
type Conversation struct {
	ThreadID     string
	UserID       string
	ChannelID    string
	StartedAt    time.Time
	LastActivity time.Time
}

var (
	conversations  = make(map[string]*Conversation) // key: threadID
	convMutex      sync.RWMutex
	discordSession *discordgo.Session
)

func loadConfig() config {
	cfg := config{
		BotToken:    os.Getenv("DISCORD_BOT_TOKEN"),
		A2AURL:      os.Getenv("KAGENT_A2A_URL"),
		MentionOnly: true,
		ClientID:    os.Getenv("DISCORD_CLIENT_ID"),
	}

	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv("DISCORD_BOT_CLIENT_ID")
	}

	if cfg.A2AURL == "" {
		log.Fatal("KAGENT_A2A_URL is required")
	}

	if v := os.Getenv("DISCORD_MENTION_ONLY"); v != "" {
		cfg.MentionOnly = strings.EqualFold(v, "true") || v == "1"
	}

	if v := os.Getenv("DISCORD_CHANNEL_ONLY"); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			if id := strings.TrimSpace(p); id != "" {
				cfg.ChannelOnly = append(cfg.ChannelOnly, id)
			}
		}
	}

	if v := os.Getenv("DISCORD_CONVERSATION_MODE"); v != "" {
		cfg.ConversationMode = v
	}
	if cfg.ConversationMode == "" {
		cfg.ConversationMode = "threaded"
	}

	if v := os.Getenv("DISCORD_PHASE_UPDATES"); v != "" {
		cfg.PhaseUpdates = strings.EqualFold(v, "true") || v == "1"
	}

	if v := os.Getenv("DISCORD_POLL_UI"); v != "" {
		cfg.PollUI = strings.EqualFold(v, "true") || v == "1"
	}

	if v := os.Getenv("DISCORD_STARTUP_CHANNEL"); v != "" {
		cfg.StartupChannel = strings.TrimSpace(v)
	}

	return cfg
}

func main() {
	cfg := loadConfig()

	if cfg.BotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is required")
	}

	// Start healthz server + outbound API for agents
	go startHTTPServer(cfg)

	// Create Discord session
	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}
	discordSession = dg

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleMessage(s, m, cfg)
	})

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	err = dg.Open()
	if err != nil {
		log.Fatalf("Error opening Discord connection: %v", err)
	}
	defer dg.Close()

	if cfg.StartupChannel != "" {
		waitForGuilds(dg, 10*time.Second)
		sendStartupMessage(dg, cfg)
	}

	dg.State.RLock()
	guildCount := len(dg.State.Guilds)
	var guildNames []string
	for _, g := range dg.State.Guilds {
		guildNames = append(guildNames, g.Name)
	}
	dg.State.RUnlock()

	if guildCount == 0 {
		log.Println("WARNING: Bot is not in any Discord server. Invite it:")
		log.Printf("  https://discord.com/oauth2/authorize?client_id=%s&permissions=309237645920&scope=bot", cfg.ClientID)
	} else {
		log.Printf("Connected to %d guild(s): %v", guildCount, guildNames)
	}

	log.Println("Kagent Discord bot is running. Press Ctrl+C to exit.")
	select {}
}

func waitForGuilds(dg *discordgo.Session, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dg.State.RLock()
		count := len(dg.State.Guilds)
		dg.State.RUnlock()
		if count > 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Println("Timed out waiting for guild data — channel name lookup may fail")
}

func sendStartupMessage(dg *discordgo.Session, cfg config) {
	if cfg.StartupChannel == "" {
		return
	}

	channelID := cfg.StartupChannel
	if _, err := strconv.ParseInt(cfg.StartupChannel, 10, 64); err != nil {
		found := findChannelByName(dg, cfg.StartupChannel)
		if found == "" {
			log.Printf("Channel not found by name: %s", cfg.StartupChannel)
			return
		}
		channelID = found
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🟢 Online — Awaiting summons",
		Description: "Loop-engineered AI workforce standing by.",
		Color:       0x00FF88,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: dg.State.User.AvatarURL("128"),
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Mention mode",
				Value:  fmt.Sprintf("`%v`", cfg.MentionOnly),
				Inline: true,
			},
			{
				Name:   "Conversations",
				Value:  fmt.Sprintf("`%s`", cfg.ConversationMode),
				Inline: true,
			},
			{
				Name:   "Phase updates",
				Value:  fmt.Sprintf("`%v`", cfg.PhaseUpdates),
				Inline: true,
			},
			{
				Name:   "A2A endpoint",
				Value:  fmt.Sprintf("`%s`", cfg.A2AURL),
				Inline: false,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "ready to work",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, sendErr := dg.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s> is online", dg.State.User.ID),
		Embed:   embed,
	})
	if sendErr != nil {
		log.Printf("Failed to send startup message to channel %s: %v", channelID, sendErr)
	} else {
		log.Printf("Startup announcement sent to channel %s", channelID)
	}
}

func findChannelByName(dg *discordgo.Session, name string) string {
	for _, guild := range dg.State.Guilds {
		channels, err := dg.GuildChannels(guild.ID)
		if err != nil {
			continue
		}
		for _, ch := range channels {
			if ch.Name == name && ch.Type == discordgo.ChannelTypeGuildText {
				return ch.ID
			}
		}
	}
	return ""
}

func startHTTPServer(cfg config) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Outbound endpoint: agents can POST status updates here
	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			ThreadID string `json:"thread_id"`
			Status   string `json:"status"`   // "working", "waiting", "done", "error"
			Agent    string `json:"agent"`    // which agent is reporting
			Message  string `json:"message"`  // human-readable status
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "bad request: %v\n", err)
			return
		}

		if payload.ThreadID == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "thread_id required")
			return
		}

		if discordSession != nil {
			msg := fmt.Sprintf("🤖 **%s** → `%s`", payload.Agent, payload.Status)
			if payload.Message != "" {
				msg += "\n" + payload.Message
			}
			if _, err := discordSession.ChannelMessageSend(payload.ThreadID, msg); err != nil {
				log.Printf("[NOTIFY] failed to send to thread %s: %v", payload.ThreadID, err)
			}
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	log.Printf("Starting HTTP server on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

func handleMessage(s *discordgo.Session, m *discordgo.MessageCreate, cfg config) {
	// Ignore own messages
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Channel filter
	if len(cfg.ChannelOnly) > 0 {
		allowed := false
		for _, id := range cfg.ChannelOnly {
			if m.ChannelID == id {
				allowed = true
				break
			}
		}
		if !allowed {
			return
		}
	}

	// Mention filter
	content := m.Content
	if cfg.MentionOnly {
		if !strings.Contains(content, fmt.Sprintf("<@%s>", s.State.User.ID)) &&
			!strings.Contains(content, fmt.Sprintf("<@!%s>", s.State.User.ID)) {
			return
		}
		content = stripMention(content, s.State.User.ID)
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	// Determine target channel/thread for reply
	replyTargetID := m.ChannelID

	// Handle conversation mode
	if m.GuildID != "" {
		// In guild - check conversation mode
		if cfg.ConversationMode == "threaded" {
			// Check if message is already in a thread we track
			convMutex.RLock()
			var existingConv *Conversation
			for _, conv := range conversations {
				if conv.ThreadID == m.ChannelID {
					existingConv = conv
					break
				}
			}
			convMutex.RUnlock()

			if existingConv != nil {
				convMutex.Lock()
				existingConv.LastActivity = time.Now()
				convMutex.Unlock()
				replyTargetID = existingConv.ThreadID
			} else {
				thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
					Name:                fmt.Sprintf("kagent-%s", truncate(content, 30)),
					AutoArchiveDuration: 60,
				})
				if err != nil {
					log.Printf("Failed to create thread: %v", err)
				} else {
					replyTargetID = thread.ID
					convMutex.Lock()
					conversations[thread.ID] = &Conversation{
						ThreadID:     thread.ID,
						UserID:       m.Author.ID,
						ChannelID:    m.ChannelID,
						StartedAt:    time.Now(),
						LastActivity: time.Now(),
					}
					convMutex.Unlock()
					log.Printf("Created thread %s for user %s", thread.ID, m.Author.Username)
				}
			}
		}
		// If mode is "dm" in guild, reply in channel (no thread)
	} else {
		// In DM - always use DM channel (natural isolation by user)
		replyTargetID = m.ChannelID
	}

	// Show typing indicator in target
	s.ChannelTyping(replyTargetID)

	// Call A2A with thread context
	reply, err := callA2A(cfg.A2AURL, content, replyTargetID)
	if err != nil {
		log.Printf("A2A call failed: %v", err)
		s.ChannelMessageSend(replyTargetID, "Sorry, I encountered an error processing your request.")
		return
	}

	// Reply in thread/channel
	_, err = s.ChannelMessageSend(replyTargetID, reply)
	if err != nil {
		log.Printf("Failed to send reply: %v", err)
	}
}

func callA2A(url, message, threadID string) (string, error) {
	req := a2aRequest{
		JSONRPC: "2.0",
		Method:  "tasks/send",
		Params: a2aParams{
			Messages: []a2aMessage{
				{Role: "user", Content: message},
			},
		},
		ID: json.Number("1"),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Pass thread ID as metadata for the agent
	httpReq.Header.Set("X-Conversation-ID", threadID)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	var a2aResp a2aResponse
	if err := json.NewDecoder(resp.Body).Decode(&a2aResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if a2aResp.Error != nil {
		return "", fmt.Errorf("A2A error: %s (code %d)", a2aResp.Error.Message, a2aResp.Error.Code)
	}

	if a2aResp.Result == nil || len(a2aResp.Result.Messages) == 0 {
		return "", fmt.Errorf("empty A2A response")
	}

	for i := len(a2aResp.Result.Messages) - 1; i >= 0; i-- {
		if a2aResp.Result.Messages[i].Role == "assistant" {
			return a2aResp.Result.Messages[i].Content, nil
		}
	}

	return "", fmt.Errorf("no assistant message in A2A response")
}

func stripMention(content, userID string) string {
	without := strings.ReplaceAll(content, fmt.Sprintf("<@%s>", userID), "")
	without = strings.ReplaceAll(without, fmt.Sprintf("<@!%s>", userID), "")
	return without
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
