package domain

type AuctionStatus string

const (
	AuctionStatusActive   AuctionStatus = "ACTIVE"
	AuctionStatusEnded    AuctionStatus = "ENDED"
	AuctionStatusArchived AuctionStatus = "ARCHIVED"
)

type BidSummary struct {
	UserName string `json:"userName"`
	Price    int64  `json:"price"`
}

type Auction struct {
	ID             string       `json:"id"`
	OwnerID        string       `json:"ownerId"`
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	ImageURLs      []string     `json:"imageUrls"`
	Status         AuctionStatus `json:"status"`
	CreatedAtMs    int64        `json:"createdAtMs"`
	ExpiresAtMs    int64        `json:"expiresAtMs"`
	EndedAtMs      int64        `json:"endedAtMs,omitempty"`
	OffersCount    int          `json:"offersCount"`
	FinalTopOffers []BidSummary `json:"finalTopOffers,omitempty"`
}

type CreateAuctionInput struct {
	OwnerID      string   `json:"ownerId"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	ImageURLs    []string `json:"imageUrls"`
	DurationHours int     `json:"durationHours"`
}

type AuctionFeedFilter struct {
	Status AuctionStatus
	Sort   string // new | trending
}
