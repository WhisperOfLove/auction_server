package domain

type AuctionFeedPage struct {
	Items      []Auction `json:"items"`
	NextCursor string    `json:"nextCursor,omitempty"`
	HasMore    bool      `json:"hasMore"`
}
