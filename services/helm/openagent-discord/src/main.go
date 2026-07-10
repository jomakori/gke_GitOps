package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	ClientID         string
	APIURL           string
	APIKey           string
	MentionOnly      bool
	ChannelOnly      []string
	ConversationMode string
	PhaseUpdates     bool
	PollUI           bool
	StartupChannel   string
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message chatMessage `json:"message"`
}

type Conversation struct {
	ThreadID     string
	UserID       string
	ChannelID    string
	StartedAt    time.Time
	LastActivity time.Time
}

var (
	conversations  = make(map[string]*Conversation)
	convMutex      sync.RWMutex
	discordSession *discordgo.Session
)

func loadConfig() config {
	cfg := config{
		BotToken:    os.Getenv("DISCORD_BOT_TOKEN"),
		MentionOnly: true,
		ClientID:    os.Getenv("DISCORD_CLIENT_ID"),
		APIURL:      os.Getenv("AGENT_API_URL"),
		APIKey:      os.Getenv("AGENT_API_KEY"),
	}

	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv("DISCORD_BOT_CLIENT_ID")
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

	if cfg.BotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is required")
	}
	if cfg.APIURL == "" {
		log.Fatal("AGENT_API_URL is required")
	}

	return cfg
}

func main() {
	cfg := loadConfig()

	go startHTTPServer(cfg)

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

	log.Println("Discord agent bot is running. Press Ctrl+C to exit.")
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
	log.Println("Timed out waiting for guild data")
}

func sendStartupMessage(dg *discordgo.Session, cfg config) {
	if cfg.StartupChannel == "" {
		return
	}

	channelID := cfg.StartupChannel
	if _, err := strconv.ParseInt(cfg.StartupChannel, 10, 64); err != nil {
		found := findChannelByName(dg, cfg.StartupChannel)
		if found == "" {
			log.Printf("Channel not found: %s", cfg.StartupChannel)
			return
		}
		channelID = found
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Online — Awaiting summons",
		Description: "Agent backend connected.",
		Color:       0x00FF88,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: dg.State.User.AvatarURL("128"),
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "API",
				Value:  fmt.Sprintf("`%s`", cfg.APIURL),
				Inline: false,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "ready",
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	_, sendErr := dg.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s> is online", dg.State.User.ID),
		Embed:   embed,
	})
	if sendErr != nil {
		log.Printf("Failed to send startup message: %v", sendErr)
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

	mux.HandleFunc("/notify", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			ThreadID string `json:"thread_id"`
			Status   string `json:"status"`
			Agent    string `json:"agent"`
			Message  string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if discordSession != nil && payload.ThreadID != "" {
			msg := fmt.Sprintf("**%s** → `%s`", payload.Agent, payload.Status)
			if payload.Message != "" {
				msg += "\n" + payload.Message
			}
			discordSession.ChannelMessageSend(payload.ThreadID, msg)
		}
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("Starting HTTP server on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

func handleMessage(s *discordgo.Session, m *discordgo.MessageCreate, cfg config) {
	if m.Author.ID == s.State.User.ID {
		return
	}

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

	replyTargetID := m.ChannelID

	if m.GuildID != "" && cfg.ConversationMode == "threaded" {
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
				Name:                fmt.Sprintf("agent-%s", truncate(content, 30)),
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
			}
		}
	}

	s.ChannelTyping(replyTargetID)

	reply, err := callAgent(cfg, content)
	if err != nil {
		log.Printf("Agent call failed: %v", err)
		s.ChannelMessageSend(replyTargetID, "Sorry, I encountered an error processing your request.")
		return
	}

	if reply != "" {
		s.ChannelMessageSend(replyTargetID, reply)
	}
}

func callAgent(cfg config, message string) (string, error) {
	req := chatRequest{
		Model: "default",
		Messages: []chatMessage{
			{Role: "user", Content: message},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	log.Printf("[agent] POST %s: %s", cfg.APIURL, truncate(message, 80))

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 256))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	result := chatResp.Choices[0].Message.Content
	log.Printf("[agent] Response (%d chars): %s", len(result), truncate(result, 100))
	return result, nil
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
