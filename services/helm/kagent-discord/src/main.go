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
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type config struct {
	BotToken         string
	A2AURL           string
	MentionOnly      bool
	ChannelOnly      []string
	ConversationMode string // "threaded" or "dm"
	PhaseUpdates     bool
	PollUI           bool
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
	conversations = make(map[string]*Conversation) // key: threadID
	convMutex     sync.RWMutex
)

func loadConfig() config {
	cfg := config{
		BotToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		A2AURL:        os.Getenv("KAGENT_A2A_URL"),
		MentionOnly:   true,
		ThreadPerConv: true, // Default: new thread per conversation
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

		// Send status update to Discord thread
		// This requires access to the discordgo session - we'd need to pass it
		// For now, just log it
		log.Printf("[NOTIFY] thread=%s agent=%s status=%s: %s",
			payload.ThreadID, payload.Agent, payload.Status, payload.Message)

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
