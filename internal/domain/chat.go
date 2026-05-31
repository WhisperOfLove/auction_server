package domain

type ChatMessage struct {
	ID            int64  `json:"id"`
	AuctionID     string `json:"auctionId"`
	SenderID      string `json:"senderId"`
	PeerID        string `json:"peerId"`
	SenderName    string `json:"senderName"`
	Body          string `json:"body"`
	AttachmentURL string `json:"attachmentUrl,omitempty"`
	CreatedAtMs   int64  `json:"createdAtMs"`
}

type SendChatInput struct {
	SenderID      string `json:"senderId"`
	PeerID        string `json:"peerId"`
	SenderName    string `json:"senderName"`
	Body          string `json:"body"`
	AttachmentURL string `json:"attachmentUrl,omitempty"`
}

type ChatReportInput struct {
	AuctionID  string `json:"auctionId"`
	ReporterID string `json:"reporterId"`
	PeerID     string `json:"peerId"`
	Body       string `json:"body"`
}

type ChatThread struct {
	AuctionID      string `json:"auctionId"`
	OwnerID        string `json:"ownerId"`
	PeerID         string `json:"peerId"`
	PeerName       string `json:"peerName"`
	PeerPhone        string `json:"peerPhone"`
	PeerReceiveCalls bool   `json:"peerReceiveCalls"`
	PeerGender       string `json:"peerGender"`
	PeerFamilyName string `json:"peerFamilyName"`
	PeerAvatarKey  string `json:"peerAvatarKey"`
	LastBody       string `json:"lastBody"`
	LastAtMs       int64  `json:"lastAtMs"`
	LastSenderID   string `json:"lastSenderId"`
}
