package domain

type Bid struct {
	AuctionID string `json:"auctionId"`
	UserName  string `json:"userName"`
	Price     int64  `json:"price"`
	CreatedAt int64  `json:"createdAt"`
}

type PlaceBidInput struct {
	UserName string `json:"userName"`
	Price    int64  `json:"price"`
}
