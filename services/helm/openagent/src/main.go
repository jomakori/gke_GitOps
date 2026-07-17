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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

var (
	k8sAPIToken   string
	k8sAPIBaseURL = "https://kubernetes.default.svc"
)

type ThinkMode int

const (
	ThinkOff ThinkMode = iota
	ThinkSimple
	ThinkFull
)

func (t ThinkMode) String() string {
	switch t {
	case ThinkOff:
		return "off"
	case ThinkSimple:
		return "simple"
	case ThinkFull:
		return "full"
	default:
		return "unknown"
	}
}

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
	AgentRef         string
	DashboardBase    string
	ThinkMode        ThinkMode
	RepoCachePath    string
}

type tokenUsage struct {
	inputTokens  int
	outputTokens int
	totalTokens  int
	durationMs   int
	toolCalls    int
	cost         float64
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
	ThreadID          string
	UserID            string
	ChannelID         string
	StartedAt         time.Time
	LastActivity      time.Time
	SessionEmbedMsgID string
	DashboardURL      string
	SessionRunID      string
}

var (
	conversations  = make(map[string]*Conversation) // key: threadID
	convMutex      sync.RWMutex
	discordSession *discordgo.Session
)

func loadConversations(path string) error {
	convMutex.Lock()
	defer convMutex.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("no state file at %s, starting fresh", path)
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}

	if err := json.Unmarshal(data, &conversations); err != nil {
		return fmt.Errorf("unmarshal state: %w", err)
	}

	log.Printf("loaded %d conversations from %s", len(conversations), path)
	return nil
}

func saveConversations(path string) error {
	convMutex.RLock()
	data, err := json.Marshal(conversations)
	convMutex.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write tmp state: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}

func loadConfig() config {
	cfg := config{
		BotToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		A2AURL:        os.Getenv("AGENT_API_URL"),
		MentionOnly:   true,
		ClientID:      os.Getenv("DISCORD_CLIENT_ID"),
		AgentRef:      getEnvDefault("AGENT_REF", "omo-loop-engineering-sisyphus"),
		DashboardBase: getEnvDefault("DASHBOARD_BASE_URL", "https://openagent.maklab.net/runs"),
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

	switch v := os.Getenv("THINK_MODE"); v {
	case "off":
		cfg.ThinkMode = ThinkOff
	case "simple":
		cfg.ThinkMode = ThinkSimple
	case "full", "":
		cfg.ThinkMode = ThinkFull
	default:
		log.Printf("WARNING: unknown THINK_MODE %q, defaulting to full", v)
		cfg.ThinkMode = ThinkFull
	}

	cfg.RepoCachePath = os.Getenv("REPO_CACHE_PATH")

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
	log.Printf("think mode: %s", cfg.ThinkMode.String())

	if cfg.BotToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN is required")
	}

	// Start healthz server + outbound API for agents
	go startHTTPServer(cfg)

	// K8s leader election for multi-replica state safety
	podName := os.Getenv("HOSTNAME")
	if podName == "" {
		podName = fmt.Sprintf("openagent-discord-%d", time.Now().UnixNano())
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to get in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to create clientset: %v", err)
	}

	lock := &ConfigMapLock{
		ConfigMapMeta: metav1.ObjectMeta{
			Name:      "openagent-discord-leader",
			Namespace: getEnvDefault("POD_NAMESPACE", "openagent"),
		},
		Client: clientset.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: podName,
		},
	}

	leaderCtx, leaderCancel := context.WithCancel(context.Background())
	defer leaderCancel()

	leaderCh := make(chan struct{})
	go func() {
		leaderelection.RunOrDie(leaderCtx, leaderelection.LeaderElectionConfig{
			Lock:            lock,
			LeaseDuration:   15 * time.Second,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(ctx context.Context) {
					log.Printf("acquired leadership as %s", podName)
					close(leaderCh)
				},
				OnStoppedLeading: func() {
					log.Printf("lost leadership, exiting")
					os.Exit(0)
				},
				OnNewLeader: func(identity string) {
					if identity != podName {
						log.Printf("new leader elected: %s (we are %s)", identity, podName)
					}
				},
			},
		})
	}()

	log.Printf("waiting for leader election (pod: %s)...", podName)
	select {
	case <-leaderCh:
		log.Printf("we are the leader! starting bot...")
	case <-time.After(60 * time.Second):
		log.Fatalf("timed out waiting for leader election after 60s")
	}

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

	if err := loadConversations("/state/conversations.json"); err != nil {
		log.Printf("WARNING: failed to load state: %v", err)
	}

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

func buildSessionEmbed(dg *discordgo.Session, conv *Conversation, usage *tokenUsage) *discordgo.MessageEmbed {
	desc := fmt.Sprintf("[Dashboard](%s)", conv.DashboardURL)
	if usage != nil {
		desc += fmt.Sprintf(" · **%d** tokens · $%.4f", usage.totalTokens, usage.cost)
	}
	return &discordgo.MessageEmbed{
		Title:       "Session",
		Description: desc,
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Started",
				Value:  fmt.Sprintf("<t:%d:R>", conv.StartedAt.Unix()),
				Inline: true,
			},
			{
				Name:   "Run",
				Value:  fmt.Sprintf("`%s`", conv.SessionRunID),
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: conv.ThreadID,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func createSessionEmbed(dg *discordgo.Session, conv *Conversation) {
	embed := buildSessionEmbed(dg, conv, nil)
	msg, err := dg.ChannelMessageSendComplex(conv.ThreadID, &discordgo.MessageSend{
		Embed: embed,
	})
	if err != nil {
		log.Printf("[Session] Failed to create embed in thread %s: %v", conv.ThreadID, err)
		return
	}

	if err := dg.ChannelMessagePin(conv.ThreadID, msg.ID); err != nil {
		log.Printf("[Session] Failed to pin embed in thread %s: %v", conv.ThreadID, err)
	}

	convMutex.Lock()
	conv.SessionEmbedMsgID = msg.ID
	convMutex.Unlock()
	go func() {
		if err := saveConversations("/state/conversations.json"); err != nil {
			log.Printf("ERROR: failed to save state: %v", err)
		}
	}()
	log.Printf("[Session] Created pinned embed %s in thread %s", msg.ID, conv.ThreadID)
}

func updateSessionEmbed(dg *discordgo.Session, conv *Conversation, usage *tokenUsage) {
	if conv.SessionEmbedMsgID == "" {
		return
	}

	embed := buildSessionEmbed(dg, conv, usage)
	_, err := dg.ChannelMessageEditEmbed(conv.ThreadID, conv.SessionEmbedMsgID, embed)
	if err != nil {
		log.Printf("[Session] Failed to update embed %s: %v", conv.SessionEmbedMsgID, err)
	}
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

	mux.HandleFunc("/search", handleSearch(cfg.RepoCachePath))

	log.Printf("Starting HTTP server on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

func isThreadChannel(s *discordgo.Session, channelID string) bool {
	ch, err := s.State.Channel(channelID)
	if err != nil {
		return false
	}
	return ch.Type == discordgo.ChannelTypeGuildPublicThread ||
		ch.Type == discordgo.ChannelTypeGuildPrivateThread ||
		ch.Type == discordgo.ChannelTypeGuildNewsThread
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

	// Check if message is in a tracked thread (skip mention check for threads)
	convMutex.RLock()
	inTrackedThread := false
	for _, conv := range conversations {
		if conv.ThreadID == m.ChannelID {
			inTrackedThread = true
			break
		}
	}
	convMutex.RUnlock()

	// Mention filter (skip if in tracked thread)
	content := m.Content
	if cfg.MentionOnly && !inTrackedThread {
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
				go func() {
					if err := saveConversations("/state/conversations.json"); err != nil {
						log.Printf("ERROR: failed to save state: %v", err)
					}
				}()
				replyTargetID = existingConv.ThreadID
			} else if isThreadChannel(s, m.ChannelID) {
				// Message in existing thread after bot restart — register it
				replyTargetID = m.ChannelID
				conv := &Conversation{
					ThreadID:     m.ChannelID,
					UserID:       m.Author.ID,
					ChannelID:    m.ChannelID,
					StartedAt:    time.Now(),
					LastActivity: time.Now(),
				}
				convMutex.Lock()
				conversations[m.ChannelID] = conv
				convMutex.Unlock()
				go func() {
					if err := saveConversations("/state/conversations.json"); err != nil {
						log.Printf("ERROR: failed to save state: %v", err)
					}
				}()
				log.Printf("Registered existing thread %s for user %s", m.ChannelID, m.Author.Username)
				createSessionEmbed(s, conv)
			} else {
				thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
					Name:                fmt.Sprintf("kagent-%s", truncate(content, 30)),
					AutoArchiveDuration: 60,
				})
				if err != nil {
					log.Printf("Failed to create thread: %v", err)
				} else {
					replyTargetID = thread.ID
					conv := &Conversation{
						ThreadID:     thread.ID,
						UserID:       m.Author.ID,
						ChannelID:    m.ChannelID,
						StartedAt:    time.Now(),
						LastActivity: time.Now(),
					}
					convMutex.Lock()
					conversations[thread.ID] = conv
					convMutex.Unlock()
					go func() {
						if err := saveConversations("/state/conversations.json"); err != nil {
							log.Printf("ERROR: failed to save state: %v", err)
						}
					}()
					log.Printf("Created thread %s for user %s", thread.ID, m.Author.Username)
					createSessionEmbed(s, conv)
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

	runID := fmt.Sprintf("discord-%s-%d", sanitizeName(replyTargetID), time.Now().Unix())

	convMutex.RLock()
	conv := conversations[replyTargetID]
	convMutex.RUnlock()

	if conv != nil {
		convMutex.Lock()
		conv.SessionRunID = runID
		conv.DashboardURL = fmt.Sprintf("%s/%s", cfg.DashboardBase, runID)
		conv.LastActivity = time.Now()
		convMutex.Unlock()
		go func() {
			if err := saveConversations("/state/conversations.json"); err != nil {
				log.Printf("ERROR: failed to save state: %v", err)
			}
		}()

		if conv.SessionEmbedMsgID == "" {
			createSessionEmbed(s, conv)
		} else {
			updateSessionEmbed(s, conv, nil)
		}
	}

	// Call Sympozium agent via Kubernetes API
	reply, usage, err := callSympoziumAPI(cfg, content, replyTargetID, runID)
	if err != nil {
		log.Printf("Sympozium call failed: %v", err)
		s.ChannelMessageSend(replyTargetID, "Sorry, I encountered an error processing your request.")
		return
	}

	if conv != nil && usage != nil {
		updateSessionEmbed(s, conv, usage)
	}

	// Reply in thread/channel
	_, err = s.ChannelMessageSend(replyTargetID, reply)
	if err != nil {
		log.Printf("Failed to send reply: %v", err)
	}
}

func formatPhaseMessage(phase string) string {
	switch phase {
	case "Running":
		return "🤔 Phase: Running"
	case "Succeeded":
		return "✅ Phase: Succeeded"
	case "Failed":
		return "❌ Phase: Failed"
	default:
		return fmt.Sprintf("🔄 Phase: %s", phase)
	}
}

func parseLogEvent(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "planning"), strings.Contains(lower, "plan:"):
		return "📝 Planning"
	case strings.Contains(lower, "executing"), strings.Contains(lower, "tool:"):
		return "🔧 Using tool"
	case strings.Contains(lower, "thinking"), strings.Contains(lower, "think:"):
		return "🤔 Thinking"
	case strings.Contains(lower, "delegate"):
		return "🔄 Delegating"
	case strings.Contains(lower, "error"), strings.Contains(lower, "error:"):
		return "❌ Error"
	default:
		return ""
	}
}

func callSympoziumAPI(cfg config, message, threadID, runID string) (string, *tokenUsage, error) {
	agentRun := map[string]interface{}{
		"apiVersion": "sympozium.ai/v1alpha1",
		"kind":       "AgentRun",
		"metadata": map[string]interface{}{
			"name":      runID,
			"namespace": "sympozium-system",
		},
		"spec": map[string]interface{}{
			"agentId":    "primary",
			"agentRef":   cfg.AgentRef,
			"task":       message,
			"mode":       "task",
			"cleanup":    "delete",
			"sessionKey": threadID,
			"model": map[string]interface{}{
				"baseURL": "http://openagent-headroom.openagent.svc.cluster.local:8787/v1",
			},
		},
	}

	body, err := json.Marshal(agentRun)
	if err != nil {
		return "", nil, fmt.Errorf("marshal run: %w", err)
	}

	log.Printf("[Sympozium] Creating AgentRun %s: %s", runID, truncate(message, 80))

	createURL := fmt.Sprintf("%s/apis/sympozium.ai/v1alpha1/namespaces/sympozium-system/agentruns", k8sAPIBaseURL)
	_, err = k8sAPIRequest(http.MethodPost, createURL, body)
	if err != nil {
		return "", nil, fmt.Errorf("create AgentRun: %w", err)
	}

	// Poll for completion (max 5 minutes)
	statusURL := fmt.Sprintf("%s/apis/sympozium.ai/v1alpha1/namespaces/sympozium-system/agentruns/%s", k8sAPIBaseURL, runID)
	var result string
	var prevPhase string
	var logStreamStarted bool

	done := make(chan struct{})
	defer close(done)

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

		// Simple think mode: phase transitions
		if cfg.ThinkMode == ThinkSimple && phase != prevPhase && prevPhase != "" {
			msg := formatPhaseMessage(phase)
			log.Printf("[ThinkMode] simple: posting phase change %q -> %q", prevPhase, phase)
			if discordSession != nil {
				discordSession.ChannelMessageSend(threadID, msg)
			} else {
				log.Printf("[ThinkMode] simple: discordSession is nil, cannot post")
			}
		}
		prevPhase = phase

		// Full think mode: pod log streaming
		if cfg.ThinkMode == ThinkFull && phase == "Running" && !logStreamStarted {
			logStreamStarted = true
			if podName, ok := status["podName"].(string); ok && podName != "" {
				log.Printf("[ThinkMode] full: starting log stream for pod %s", podName)
				go func(pod string) {
					msgCount := 0
					ticker := time.NewTicker(3 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-done:
							return
						case <-ticker.C:
						}
						logs, err := getPodLogs(pod)
						if err != nil {
							log.Printf("[ThinkMode] log stream error: %v", err)
							continue
						}
						for _, line := range strings.Split(logs, "\n") {
							if msgCount >= 10 {
								return
							}
							if label := parseLogEvent(line); label != "" {
								log.Printf("[ThinkMode] full: posting event %q", label)
								if discordSession != nil {
									discordSession.ChannelMessageSend(threadID, label)
								}
								msgCount++
								if msgCount >= 10 {
									return
								}
							}
						}
						// Check if run finished
						checkResp, checkErr := k8sAPIRequest("GET",
							fmt.Sprintf("%s/apis/sympozium.ai/v1alpha1/namespaces/sympozium-system/agentruns/%s", k8sAPIBaseURL, runID), nil)
						if checkErr == nil {
							var checkStatus map[string]interface{}
							if json.Unmarshal(checkResp, &checkStatus) == nil {
								if s, ok := checkStatus["status"].(map[string]interface{}); ok {
									if p, _ := s["phase"].(string); p == "Succeeded" || p == "Failed" {
										return
									}
								}
							}
						}
					}
				}(podName)
			}
		}

		switch phase {
		case "Succeeded":
			result, _ = status["result"].(string)
			if result == "" {
				podName, _ := status["podName"].(string)
				if podName != "" {
					result, err = getPodLogs(podName)
					if err != nil {
						log.Printf("[Sympozium] pod logs unavailable for %s: %v, returning empty result", podName, err)
						result = "Agent run completed successfully."
					}
				} else {
					result = "Agent run completed successfully."
				}
			}

			usage := parseTokenUsage(status)
			log.Printf("[Sympozium] Response (%d chars): %s", len(result), truncate(result, 100))
			return result, usage, nil

		case "Failed":
			errMsg, _ := status["error"].(string)
			return "", nil, fmt.Errorf("AgentRun failed: %s", errMsg)
		}
	}

	return "", nil, fmt.Errorf("timed out waiting for AgentRun completion")
}

func parseTokenUsage(status map[string]interface{}) *tokenUsage {
	tu, ok := status["tokenUsage"].(map[string]interface{})
	if !ok {
		return nil
	}

	u := &tokenUsage{}
	if v, ok := tu["inputTokens"].(float64); ok {
		u.inputTokens = int(v)
	}
	if v, ok := tu["outputTokens"].(float64); ok {
		u.outputTokens = int(v)
	}
	if v, ok := tu["totalTokens"].(float64); ok {
		u.totalTokens = int(v)
	}
	if v, ok := tu["durationMs"].(float64); ok {
		u.durationMs = int(v)
	}
	if v, ok := tu["toolCalls"].(float64); ok {
		u.toolCalls = int(v)
	}

	// Approximate cost: Claude Sonnet 4 pricing ($3/1M input, $15/1M output)
	u.cost = float64(u.inputTokens)*3.0/1_000_000 + float64(u.outputTokens)*15.0/1_000_000

	return u
}

func getPodLogs(podName string) (string, error) {
	logsURL := fmt.Sprintf("%s/api/v1/namespaces/sympozium-system/pods/%s/log?container=agent", k8sAPIBaseURL, podName)
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
