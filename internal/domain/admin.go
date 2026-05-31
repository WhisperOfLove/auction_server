package domain

type AdminTicketThread struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
	LastBody    string `json:"lastBody"`
	LastAtMs    int64  `json:"lastAtMs"`
	LastSender  string `json:"lastSender"`
}

type AdminTicketMessageInput struct {
	UserID        string `json:"userId"`
	Body          string `json:"body"`
	AttachmentURL string `json:"attachmentUrl"`
}

type AdminUserRow struct {
	ID                     string `json:"id"`
	UserNumber             int64  `json:"userNumber"`
	Username               string `json:"username"`
	Phone                  string `json:"phone"`
	SubscriptionPlan       string `json:"subscriptionPlan"`
	RegisteredAtMs         int64  `json:"registeredAtMs"`
	PostCount              int64  `json:"postCount"`
	SubscriptionBuyAtMs    int64  `json:"subscriptionBuyAtMs"`
	SubscriptionExpireAtMs int64  `json:"subscriptionExpireAtMs"`
}

type AdminUsersPage struct {
	Items []AdminUserRow `json:"items"`
	Total int64          `json:"total"`
}

type AdminPeriodCount struct {
	Label string `json:"label"`
	Value int64  `json:"value"`
}

type AdminReportsResponse struct {
	Posts         []AdminPeriodCount `json:"posts"`
	Registrations []AdminPeriodCount `json:"registrations"`
}
