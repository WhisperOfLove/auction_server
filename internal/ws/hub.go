package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const supportAuctionID = "__support__"

type Hub struct {
	mu          sync.RWMutex
	rooms       map[string]map[*websocket.Conn]struct{}
	adminAPIKey string
}

func NewHub(adminAPIKey string) *Hub {
	return &Hub{
		rooms:       make(map[string]map[*websocket.Conn]struct{}),
		adminAPIKey: strings.TrimSpace(adminAPIKey),
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/v1/ws/auctions/"):
		h.serveAuctionBids(w, r)
	case strings.HasPrefix(path, "/v1/ws/chat/threads/"):
		h.serveChatThread(w, r)
	case path == "/v1/ws/chat/inbox":
		h.serveChatInbox(w, r)
	case path == "/v1/ws/admin/support-tickets":
		h.serveAdminSupportTickets(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Hub) serveAuctionBids(w http.ResponseWriter, r *http.Request) {
	auctionID := strings.TrimPrefix(r.URL.Path, "/v1/ws/auctions/")
	auctionID = strings.TrimSuffix(auctionID, "/bids")
	auctionID = strings.Trim(auctionID, "/")
	if auctionID == "" {
		http.Error(w, "auction id required", http.StatusBadRequest)
		return
	}
	h.serveRoom(w, r, auctionID)
}

func (h *Hub) serveChatThread(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/ws/chat/threads/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 3 {
		http.Error(w, "path: /v1/ws/chat/threads/{auctionId}/{viewerId}/{peerId}", http.StatusBadRequest)
		return
	}
	room := threadRoom(parts[0], parts[1], parts[2])
	h.serveRoom(w, r, room)
}

func (h *Hub) serveChatInbox(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	if userID == "" {
		http.Error(w, "userId query required", http.StatusBadRequest)
		return
	}
	h.serveRoom(w, r, inboxRoom(userID))
}

func threadRoom(auctionID, viewerID, peerID string) string {
	return "thread:" + auctionID + ":" + viewerID + ":" + peerID
}

func inboxRoom(userID string) string {
	return "inbox:" + userID
}

func adminSupportRoom() string {
	return "admin:support"
}

func (h *Hub) serveAdminSupportTickets(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminKey(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.serveRoom(w, r, adminSupportRoom())
}

func (h *Hub) checkAdminKey(r *http.Request) bool {
	key := h.adminAPIKey
	if key == "" {
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if got == "" {
		got = strings.TrimSpace(r.URL.Query().Get("adminKey"))
	}
	return got == key
}

func (h *Hub) serveRoom(w http.ResponseWriter, r *http.Request, room string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.join(room, conn)
	defer func() {
		h.leave(room, conn)
		_ = conn.Close()
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *Hub) join(room string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*websocket.Conn]struct{})
	}
	h.rooms[room][conn] = struct{}{}
}

func (h *Hub) leave(room string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	roomConns := h.rooms[room]
	delete(roomConns, conn)
	if len(roomConns) == 0 {
		delete(h.rooms, room)
	}
}

func (h *Hub) broadcast(room string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.rooms[room] {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			log.Printf("ws write room=%s: %v", room, err)
		}
	}
}

// NotifyTopBids pushes a lightweight event so clients refetch top bids.
func (h *Hub) NotifyTopBids(auctionID string) {
	payload, _ := json.Marshal(map[string]string{
		"type":      "top_bids_updated",
		"auctionId": auctionID,
	})
	h.broadcast(auctionID, payload)
}

// NotifyChat tells thread viewers and both users' inbox listeners to refetch via HTTP.
func (h *Hub) NotifyChat(auctionID, senderID, peerID string) {
	threadPayload, _ := json.Marshal(map[string]string{
		"type":      "chat_messages_updated",
		"auctionId": auctionID,
	})
	h.broadcast(threadRoom(auctionID, senderID, peerID), threadPayload)
	h.broadcast(threadRoom(auctionID, peerID, senderID), threadPayload)

	inboxPayload, _ := json.Marshal(map[string]string{
		"type": "inbox_updated",
	})
	h.broadcast(inboxRoom(senderID), inboxPayload)
	h.broadcast(inboxRoom(peerID), inboxPayload)

	if auctionID == supportAuctionID {
		adminPayload, _ := json.Marshal(map[string]string{
			"type": "support_tickets_updated",
		})
		h.broadcast(adminSupportRoom(), adminPayload)
	}
}
