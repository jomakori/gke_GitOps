package main

import (
	"bytes"
	"context"
	"crypto/tls"
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

var (
	k8sAPIToken   string
	k8sAPIBaseURL = "https://kubernetes.default.svc"
)

type config struct {
	BotToken         string
	A2AURL           string
	ClientID         string
	MentionOnly      bool
	ChannelOnly      []string
	ConversationMode string
	PhaseUpdates     bool
	PollUI           bool
	StartupChannel   string
	AgentID          string
	AgentRef         string
	ModelName        string
	ModelProvider    string
	AgentSkills      string
}

type a2aRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      json.Number `json:"id"`
}

type a2aMessageSendParams struct {
	Message a2aMessagePart `json:"message"`
}

type a2aMessagePart struct {
	Role      string    `json:"role"`
	Parts     []a2aPart `json:"parts"`
	ContextID string    `json:"contextId,omitempty"`
}

type a2aPart struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type a2aResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *a2aError   `json:"error,omitempty"`
	ID      json.Number `json:"id"`
}

type a2aResultTask struct {
	Kind    string           `json:"kind"`
	Status  a2aResultStatus  `json:"status"`
	ContextID string         `json:"contextId,omitempty"`
}

type a2aResultStatus struct {
	State   string        `json:"state"`
	Message *a2aSSEMessage `json:"message,omitempty"`
}

type a2aSSEMessage struct {
	Parts []a2aPart `json:"parts"`
	Role  string    `json:"role"`
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
		BotToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		A2AURL:        os.Getenv("AGENT_API_URL"),
		MentionOnly:   true,
		ClientID:      os.Getenv("DISCORD_CLIENT_ID"),
		AgentID:       getEnvDefault("AGENT_ID", "primary"),
		AgentRef:      getEnvDefault("AGENT_REF", "omo-loop-engineering-sisyphus"),
		ModelName:     getEnvDefault("MODEL_NAME", "claude/sonnet-4"),
		ModelProvider: getEnvDefault("MODEL_PROVIDER", "openai"),
		AgentSkills:   getEnvDefault("AGENT_SKILLS", "k8s-ops,omo-core-skills,hashline-editor,web-endpoint"),
	}

	if cfg.ClientID == "" {
		cfg.ClientID = os.Getenv("DISCORD_BOT_CLIENT_ID")
	}

	if cfg.A2AURL == "" {
		log.Fatal("AGENT_API_URL is required")
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

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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

	// Call Sympozium agent via Kubernetes API
	reply, err := callSympoziumAPI(cfg, content, replyTargetID)
	if err != nil {
		log.Printf("Sympozium call failed: %v", err)
		s.ChannelMessageSend(replyTargetID, "Sorry, I encountered an error processing your request.")
		return
	}

	// Reply in thread/channel
	_, err = s.ChannelMessageSend(replyTargetID, reply)
	if err != nil {
		log.Printf("Failed to send reply: %v", err)
	}
}

func callSympoziumAPI(cfg config, message, threadID string) (string, error) {
	runID := fmt.Sprintf("discord-%s-%d", sanitizeName(threadID), time.Now().Unix())

	agentRun := map[string]interface{}{
		"apiVersion": "sympozium.ai/v1alpha1",
		"kind":       "AgentRun",
		"metadata": map[string]interface{}{
			"name":      runID,
			"namespace": "sympozium-system",
		},
		"spec": map[string]interface{}{
			"agentId":    cfg.AgentID,
			"agentRef":   cfg.AgentRef,
			"task":       message,
			"mode":       "task",
			"cleanup":    "delete",
			"sessionKey": threadID,
			"model": map[string]interface{}{
				"model":    cfg.ModelName,
				"provider": cfg.ModelProvider,
			},
		},
	}

	if cfg.AgentSkills != "" {
		skills := []map[string]string{}
		for _, name := range strings.Split(cfg.AgentSkills, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				skills = append(skills, map[string]string{"skillPackRef": name})
			}
		}
		agentRun["spec"].(map[string]interface{})["skills"] = skills
	}

	body, err := json.Marshal(agentRun)
	if err != nil {
		return "", fmt.Errorf("marshal run: %w", err)
	}

	log.Printf("[Sympozium] Creating AgentRun %s: %s", runID, truncate(message, 80))

	createURL := fmt.Sprintf("%s/apis/sympozium.ai/v1alpha1/namespaces/sympozium-system/agentruns", k8sAPIBaseURL)
	_, err = k8sAPIRequest(http.MethodPost, createURL, body)
	if err != nil {
		return "", fmt.Errorf("create AgentRun: %w", err)
	}

	// Poll for completion (max 5 minutes)
	statusURL := fmt.Sprintf("%s/apis/sympozium.ai/v1alpha1/namespaces/sympozium-system/agentruns/%s", k8sAPIBaseURL, runID)
	var result string

	for i := 0; i < 60; i++ {
		time.Sleep(5 * time.Second)

		statusResp, err := k8sAPIRequest(http.MethodGet, statusURL, nil)
		if err != nil {
			log.Printf("[Sympozium] Poll error: %v", err)
			continue
		}

		var runStatus map[string]interface{}
		if err := json.Unmarshal(statusResp, &runStatus); err != nil {
			continue
		}

		status, _ := runStatus["status"].(map[string]interface{})
		if status == nil {
			continue
		}

		phase, _ := status["phase"].(string)
		log.Printf("[Sympozium] %s phase: %s", runID, phase)

		switch phase {
		case "Succeeded":
			result, _ = status["result"].(string)
			if result == "" {
				// Fallback: try pod logs if result field is empty
				podName, _ := status["podName"].(string)
				if podName != "" {
					result, err = getPodLogs(podName)
					if err != nil {
						return "", fmt.Errorf("get logs: %w", err)
					}
				}
			}
			log.Printf("[Sympozium] Response (%d chars): %s", len(result), truncate(result, 100))
			return result, nil

		case "Failed":
			errMsg, _ := status["error"].(string)
			return "", fmt.Errorf("AgentRun failed: %s", errMsg)
		}
	}

	return "", fmt.Errorf("timed out waiting for AgentRun completion")
}

func getPodLogs(podName string) (string, error) {
	logsURL := fmt.Sprintf("%s/api/v1/namespaces/sympozium-system/pods/%s/log", k8sAPIBaseURL, podName)
	data, err := k8sAPIRequest(http.MethodGet, logsURL, nil)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func k8sAPIRequest(method, url string, body []byte) ([]byte, error) {
	if k8sAPIToken == "" {
		tokenData, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		if err != nil {
			return nil, fmt.Errorf("read SA token: %w", err)
		}
		k8sAPIToken = string(tokenData)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+k8sAPIToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("K8s API %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 256)]))
	}

	return respBody, nil
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Trim(s, "-")
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
