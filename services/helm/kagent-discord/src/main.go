package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type config struct {
	BotToken    string
	A2AURL      string
	MentionOnly bool
	ChannelOnly []string
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

func loadConfig() config {
	cfg := config{
		BotToken:    os.Getenv("DISCORD_BOT_TOKEN"),
		A2AURL:      os.Getenv("KAGENT_A2A_URL"),
		MentionOnly: true,
	}

	if cfg.A2AURL == "" {
		cfg.A2AURL = "http://kagent-service.kagent:8083/api/a2a/kagent/sisyphus"
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

	return cfg
}

func main() {
	cfg := loadConfig()

	if cfg.BotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is required")
	}

	// Start healthz server
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ok")
		})
		log.Printf("Starting healthz server on :8080")
		if err := http.ListenAndServe(":8080", mux); err != nil {
			log.Fatalf("healthz server failed: %v", err)
		}
	}()

	// Create Discord session
	dg, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleMessage(s, m, cfg)
	})

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	err = dg.Open()
	if err != nil {
		log.Fatalf("Error opening Discord connection: %v", err)
	}
	defer dg.Close()

	log.Println("Kagent Discord bot is running. Press Ctrl+C to exit.")
	select {}
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
		// Strip mention from content before sending to A2A
		content = stripMention(content, s.State.User.ID)
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	// Show typing indicator
	s.ChannelTyping(m.ChannelID)

	// Call A2A
	reply, err := callA2A(cfg.A2AURL, content)
	if err != nil {
		log.Printf("A2A call failed: %v", err)
		s.ChannelMessageSend(m.ChannelID, "Sorry, I encountered an error processing your request.")
		return
	}

	// Reply in thread if in thread, otherwise in channel
	if m.GuildID != "" {
		_, err = s.ChannelMessageSendReply(m.ChannelID, reply, m.Reference())
	} else {
		_, err = s.ChannelMessageSend(m.ChannelID, reply)
	}
	if err != nil {
		log.Printf("Failed to send reply: %v", err)
	}
}

func stripMention(content, userID string) string {
	without := strings.ReplaceAll(content, fmt.Sprintf("<@%s>", userID), "")
	without = strings.ReplaceAll(without, fmt.Sprintf("<@!%s>", userID), "")
	return without
}

func callA2A(url, message string) (string, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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

	// Return the last assistant message content
	for i := len(a2aResp.Result.Messages) - 1; i >= 0; i-- {
		if a2aResp.Result.Messages[i].Role == "assistant" {
			return a2aResp.Result.Messages[i].Content, nil
		}
	}

	return "", fmt.Errorf("no assistant message in A2A response")
}
