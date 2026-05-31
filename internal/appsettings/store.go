package appsettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"auction_server/internal/config"
)

// Settings are persisted for admin panel (feature toggles + runtime app config).
type Settings struct {
	NewPostEnabled  bool   `json:"newPostEnabled"`
	MessagesEnabled bool   `json:"messagesEnabled"`
	PublicBaseURL   string `json:"publicBaseUrl"`

	BidCooldownSeconds         int    `json:"bidCooldownSeconds"`
	PostDurationHours          int    `json:"postDurationHours"`
	LastMomentHours            int    `json:"lastMomentHours"`
	ExtendDurationHours        int    `json:"extendDurationHours"`
	TopBidsPollIntervalSeconds int    `json:"topBidsPollIntervalSeconds"`
	ChatPollIntervalSeconds    int    `json:"chatPollIntervalSeconds"`
	FeedPageSize               int    `json:"feedPageSize"`
	OfflineMessage             string `json:"offlineMessage"`
}

// Provider persists and reads mobile app settings.
type Provider interface {
	Get() Settings
	Apply(patch Settings) (Settings, error)
	EffectiveBaseURL(fallback string) string
}

// DefaultsFromConfig seeds DB/file store from env-backed server config.
func DefaultsFromConfig(cfg config.Config) Settings {
	return Settings{
		NewPostEnabled:             true,
		MessagesEnabled:            true,
		PublicBaseURL:              strings.TrimSpace(cfg.PublicBaseURL),
		BidCooldownSeconds:         cfg.BidCooldownSeconds,
		PostDurationHours:          72,
		LastMomentHours:            3,
		ExtendDurationHours:        72,
		TopBidsPollIntervalSeconds: 30,
		ChatPollIntervalSeconds:    30,
		FeedPageSize:               30,
		OfflineMessage:             "دسترسی خود را به اینترنت بررسی کنید",
	}
}

// Resolved returns effective values (stored settings with env fallbacks for zeros).
func Resolved(st Settings, cfg config.Config) Settings {
	def := DefaultsFromConfig(cfg)
	out := st
	if out.BidCooldownSeconds <= 0 {
		out.BidCooldownSeconds = def.BidCooldownSeconds
	}
	if out.PostDurationHours <= 0 {
		out.PostDurationHours = def.PostDurationHours
	}
	if out.LastMomentHours <= 0 {
		out.LastMomentHours = def.LastMomentHours
	}
	if out.ExtendDurationHours <= 0 {
		out.ExtendDurationHours = def.ExtendDurationHours
	}
	if out.TopBidsPollIntervalSeconds <= 0 {
		out.TopBidsPollIntervalSeconds = def.TopBidsPollIntervalSeconds
	}
	if out.ChatPollIntervalSeconds <= 0 {
		out.ChatPollIntervalSeconds = def.ChatPollIntervalSeconds
	}
	if out.FeedPageSize <= 0 {
		out.FeedPageSize = def.FeedPageSize
	}
	if strings.TrimSpace(out.OfflineMessage) == "" {
		out.OfflineMessage = def.OfflineMessage
	}
	return out
}

type Store struct {
	mu      sync.RWMutex
	path    string
	current Settings
}

func NewStore(path string, defaults Settings) *Store {
	s := &Store{path: path, current: defaults}
	s.load()
	return s
}

var _ Provider = (*Store)(nil)

func (s *Store) load() {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var loaded Settings
	if json.Unmarshal(raw, &loaded) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = mergeSettings(s.current, loaded)
}

func mergeSettings(base, patch Settings) Settings {
	out := base
	out.NewPostEnabled = patch.NewPostEnabled
	out.MessagesEnabled = patch.MessagesEnabled
	if strings.TrimSpace(patch.PublicBaseURL) != "" {
		out.PublicBaseURL = strings.TrimSpace(patch.PublicBaseURL)
	}
	if patch.BidCooldownSeconds > 0 {
		out.BidCooldownSeconds = patch.BidCooldownSeconds
	}
	if patch.PostDurationHours > 0 {
		out.PostDurationHours = patch.PostDurationHours
	}
	if patch.LastMomentHours > 0 {
		out.LastMomentHours = patch.LastMomentHours
	}
	if patch.ExtendDurationHours > 0 {
		out.ExtendDurationHours = patch.ExtendDurationHours
	}
	if patch.TopBidsPollIntervalSeconds > 0 {
		out.TopBidsPollIntervalSeconds = patch.TopBidsPollIntervalSeconds
	}
	if patch.ChatPollIntervalSeconds > 0 {
		out.ChatPollIntervalSeconds = patch.ChatPollIntervalSeconds
	}
	if patch.FeedPageSize > 0 {
		out.FeedPageSize = patch.FeedPageSize
	}
	if strings.TrimSpace(patch.OfflineMessage) != "" {
		out.OfflineMessage = strings.TrimSpace(patch.OfflineMessage)
	}
	return out
}

func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Store) Apply(patch Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = patch
	if err := s.persistLocked(); err != nil {
		return s.current, err
	}
	return s.current, nil
}

func (s *Store) persistLocked() error {
	dir := filepath.Dir(s.path)
	if dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	b, err := json.MarshalIndent(s.current, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

func (s *Store) EffectiveBaseURL(fallback string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if u := strings.TrimSpace(s.current.PublicBaseURL); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	return strings.TrimSuffix(strings.TrimSpace(fallback), "/")
}
