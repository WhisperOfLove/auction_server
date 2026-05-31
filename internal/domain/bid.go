package domain

type Bid struct {
	AuctionID  string `json:"auctionId"`
	UserID     string `json:"userId"`
	UserName   string `json:"userName"`
	Phone      string `json:"phone"`
	Gender     string `json:"gender,omitempty"`
	FamilyName   string `json:"familyName,omitempty"`
	ReceiveCalls bool   `json:"receiveCalls"`
	Price        int64  `json:"price"`
	CreatedAt  int64  `json:"createdAt"`
}

type PlaceBidInput struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	Phone    string `json:"phone"`
	Price    int64  `json:"price"`
}

type BidCooldown struct {
	CanBid        bool  `json:"canBid"`
	NextBidAtMs   int64 `json:"nextBidAtMs"`
	RemainingMs   int64 `json:"remainingMs"`
	CooldownMs    int64 `json:"cooldownMs"`
	LastBidAtMs   int64 `json:"lastBidAtMs"`
}

type BidSummary struct {
	UserName string `json:"userName"`
	Phone    string `json:"phone"`
	Price    int64  `json:"price"`
}
