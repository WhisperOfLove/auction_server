package domain

type AuctionStatus string

const (
	AuctionStatusActive   AuctionStatus = "ACTIVE"
	AuctionStatusEnded    AuctionStatus = "ENDED"
	AuctionStatusArchived AuctionStatus = "ARCHIVED"
)

type Auction struct {
	ID             string       `json:"id"`
	OwnerID        string       `json:"ownerId"`
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	ImageURLs      []string     `json:"imageUrls"`
	Status           AuctionStatus    `json:"status"`
	ModerationStatus ModerationStatus `json:"moderationStatus"`
	ModerationReason string           `json:"moderationReason,omitempty"`
	CreatedAtMs      int64            `json:"createdAtMs"`
	ExpiresAtMs    int64        `json:"expiresAtMs"`
	EndedAtMs      int64        `json:"endedAtMs,omitempty"`
	OffersCount    int          `json:"offersCount"`
	BasePrice      int64        `json:"basePrice"`
	MinBidStep     int64        `json:"minBidStep"`
	BumpedAtMs     int64        `json:"bumpedAtMs,omitempty"`
	IsFeatured     bool         `json:"isFeatured"`
	ViewCount      int          `json:"viewCount"`
	TopBids        []Bid        `json:"topBids,omitempty"`
	FinalTopOffers []BidSummary `json:"finalTopOffers,omitempty"`
}

type CreateAuctionInput struct {
	OwnerID      string   `json:"ownerId"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	ImageURLs    []string `json:"imageUrls"`
	DurationHours int   `json:"durationHours"`
	BasePrice     int64 `json:"basePrice"`
}

type UpdateAuctionInput struct {
	OwnerID     string   `json:"ownerId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ImageURLs   []string `json:"imageUrls"`
	BasePrice   int64    `json:"basePrice"`
}

type AuctionStats struct {
	ViewCount      int `json:"viewCount"`
	OfferUserCount int `json:"offerUserCount"`
	OffersCount    int `json:"offersCount"`
}

type AuctionFeedFilter struct {
	Status AuctionStatus
	Sort   string // new | trending
	Limit  int    // 0 = legacy full list
	Cursor string // keyset: "createdAtMs:auctionId"
	BidderUserID string // when set, only auctions this user bid on
}
