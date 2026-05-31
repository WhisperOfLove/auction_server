package http

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"auction_server/internal/appsettings"
	"auction_server/internal/cache"
)

func (s *Server) effectivePublicBaseURL() string {
	if s.settings != nil {
		return s.settings.EffectiveBaseURL(s.cfg.PublicBaseURL)
	}
	return strings.TrimSuffix(strings.TrimSpace(s.cfg.PublicBaseURL), "/")
}

func (s *Server) resolvedSettings() appsettings.Settings {
	st := appsettings.DefaultsFromConfig(s.cfg)
	if s.settings != nil {
		st = appsettings.Resolved(s.settings.Get(), s.cfg)
	}
	return st
}

func settingsToPublicJSON(st appsettings.Settings, base string) map[string]any {
	wsURL := ""
	if base != "" {
		wsURL = strings.Replace(base, "https://", "wss://", 1)
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	}
	return map[string]any{
		"newPostEnabled":             st.NewPostEnabled,
		"messagesEnabled":            st.MessagesEnabled,
		"publicBaseUrl":              base,
		"wsBaseUrl":                  strings.TrimSuffix(wsURL, "/"),
		"bidCooldownSeconds":         st.BidCooldownSeconds,
		"postDurationHours":          st.PostDurationHours,
		"lastMomentHours":            st.LastMomentHours,
		"extendDurationHours":        st.ExtendDurationHours,
		"topBidsPollIntervalSeconds": st.TopBidsPollIntervalSeconds,
		"chatPollIntervalSeconds":    st.ChatPollIntervalSeconds,
		"feedPageSize":               st.FeedPageSize,
		"offlineMessage":             st.OfflineMessage,
	}
}

func (s *Server) handleAppConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	st := s.resolvedSettings()
	base := s.effectivePublicBaseURL()
	log.Printf("app_config_http_get new_post=%t messages=%t bid_cd=%d post_h=%d client=%s",
		st.NewPostEnabled, st.MessagesEnabled, st.BidCooldownSeconds, st.PostDurationHours, r.RemoteAddr)
	writeJSON(w, http.StatusOK, settingsToPublicJSON(st, base))
}

func (s *Server) handleAdminAppSettings(w http.ResponseWriter, r *http.Request) {
	if !s.adminCORS(w, r) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	if s.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings not available")
		return
	}
	switch r.Method {
	case http.MethodGet:
		st := s.resolvedSettings()
		out := settingsToPublicJSON(st, s.effectivePublicBaseURL())
		out["defaultBaseUrl"] = strings.TrimSuffix(strings.TrimSpace(s.cfg.PublicBaseURL), "/")
		out["defaults"] = settingsToPublicJSON(appsettings.DefaultsFromConfig(s.cfg), out["defaultBaseUrl"].(string))
		writeJSON(w, http.StatusOK, out)
	case http.MethodPut:
		var body struct {
			NewPostEnabled  *bool  `json:"newPostEnabled"`
			MessagesEnabled *bool  `json:"messagesEnabled"`
			PublicBaseURL   string `json:"publicBaseUrl"`

			BidCooldownSeconds         *int    `json:"bidCooldownSeconds"`
			PostDurationHours          *int    `json:"postDurationHours"`
			LastMomentHours            *int    `json:"lastMomentHours"`
			ExtendDurationHours        *int    `json:"extendDurationHours"`
			TopBidsPollIntervalSeconds *int    `json:"topBidsPollIntervalSeconds"`
			ChatPollIntervalSeconds    *int    `json:"chatPollIntervalSeconds"`
			FeedPageSize               *int    `json:"feedPageSize"`
			OfflineMessage             *string `json:"offlineMessage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		cur := s.resolvedSettings()
		if body.NewPostEnabled != nil {
			cur.NewPostEnabled = *body.NewPostEnabled
		}
		if body.MessagesEnabled != nil {
			cur.MessagesEnabled = *body.MessagesEnabled
		}
		if body.PublicBaseURL != "" {
			cur.PublicBaseURL = strings.TrimSuffix(strings.TrimSpace(body.PublicBaseURL), "/")
		}
		if body.BidCooldownSeconds != nil && *body.BidCooldownSeconds >= 0 {
			cur.BidCooldownSeconds = *body.BidCooldownSeconds
		}
		if body.PostDurationHours != nil && *body.PostDurationHours > 0 {
			cur.PostDurationHours = *body.PostDurationHours
		}
		if body.LastMomentHours != nil && *body.LastMomentHours > 0 {
			cur.LastMomentHours = *body.LastMomentHours
		}
		if body.ExtendDurationHours != nil && *body.ExtendDurationHours > 0 {
			cur.ExtendDurationHours = *body.ExtendDurationHours
		}
		if body.TopBidsPollIntervalSeconds != nil && *body.TopBidsPollIntervalSeconds > 0 {
			cur.TopBidsPollIntervalSeconds = *body.TopBidsPollIntervalSeconds
		}
		if body.ChatPollIntervalSeconds != nil && *body.ChatPollIntervalSeconds > 0 {
			cur.ChatPollIntervalSeconds = *body.ChatPollIntervalSeconds
		}
		if body.FeedPageSize != nil && *body.FeedPageSize > 0 {
			cur.FeedPageSize = *body.FeedPageSize
		}
		if body.OfflineMessage != nil {
			cur.OfflineMessage = strings.TrimSpace(*body.OfflineMessage)
		}
		st, err := s.settings.Apply(cur)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not save settings")
			return
		}
		st = appsettings.Resolved(st, s.cfg)
		cache.InvalidateFeedCache()
		log.Printf("app_config_http_put_saved new_post=%t messages=%t bid_cd=%d", st.NewPostEnabled, st.MessagesEnabled, st.BidCooldownSeconds)
		writeJSON(w, http.StatusOK, settingsToPublicJSON(st, s.effectivePublicBaseURL()))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
