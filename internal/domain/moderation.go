package domain

type ModerationStatus string

const (
	ModerationPending  ModerationStatus = "PENDING"
	ModerationApproved ModerationStatus = "APPROVED"
	ModerationRejected ModerationStatus = "REJECTED"
)

type ModerationQueueFilter struct {
	Status ModerationStatus
	Limit  int
	Cursor string // createdAtMs:auctionId
}

type ModerationQueuePage struct {
	Items      []Auction `json:"items"`
	NextCursor string    `json:"nextCursor,omitempty"`
	HasMore    bool      `json:"hasMore"`
	PendingCount int     `json:"pendingCount,omitempty"`
}

type ModerationDecisionInput struct {
	Reason string `json:"reason"`
}

type UserNotification struct {
	ID           int64  `json:"id"`
	UserID       string `json:"userId"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	RefAuctionID string `json:"refAuctionId,omitempty"`
	ReadAtMs     int64  `json:"readAtMs,omitempty"`
	CreatedAtMs  int64  `json:"createdAtMs"`
}
