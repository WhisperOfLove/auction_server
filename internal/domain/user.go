package domain

import (
	"encoding/json"
	"strings"
)

type UpsertUserInput struct {
	ID    string `json:"id"`
	Phone string `json:"phone"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// User: CustomName is stored in DB (display_name); Name is the public label (json "name").
// Username is login-only and omitted from default JSON.
type User struct {
	ID                     string `json:"id"`
	UserNumber             int64  `json:"userNumber"`
	Username               string `json:"-"`
	CustomName             string `json:"-"`
	Name                   string `json:"name"`
	Gender                 string `json:"gender"`
	AccountType            string `json:"accountType"`
	Phone                  string `json:"phone"`
	City                   string `json:"city"`
	AvatarKey              string `json:"avatarKey"`
	ReceiveCalls           bool   `json:"receiveCalls"`
	SubscriptionPlan       string `json:"subscriptionPlan"`
	SubscriptionBuyAtMs    int64  `json:"subscriptionBuyAtMs,omitempty"`
	SubscriptionExpireAtMs int64  `json:"subscriptionExpireAtMs,omitempty"`
	Bio                    string `json:"bio"`
	Verified               bool   `json:"verified"`
}

// MarshalJSON exposes name (resolved) and login username only when set.
func (u User) MarshalJSON() ([]byte, error) {
	type userAlias struct {
		ID                     string `json:"id"`
		UserNumber             int64  `json:"userNumber"`
		Username               string `json:"username,omitempty"`
		Name                   string `json:"name"`
		Gender                 string `json:"gender"`
		AccountType            string `json:"accountType"`
		Phone                  string `json:"phone"`
		City                   string `json:"city"`
		AvatarKey              string `json:"avatarKey"`
		ReceiveCalls           bool   `json:"receiveCalls"`
		SubscriptionPlan       string `json:"subscriptionPlan"`
		SubscriptionBuyAtMs    int64  `json:"subscriptionBuyAtMs,omitempty"`
		SubscriptionExpireAtMs int64  `json:"subscriptionExpireAtMs,omitempty"`
		Bio                    string `json:"bio"`
		Verified               bool   `json:"verified"`
	}
	out := userAlias{
		ID:                     u.ID,
		UserNumber:             u.UserNumber,
		Name:                   u.resolvedName(),
		Gender:                 u.Gender,
		AccountType:            u.AccountType,
		Phone:                  u.Phone,
		City:                   u.City,
		AvatarKey:              u.AvatarKey,
		ReceiveCalls:           u.ReceiveCalls,
		SubscriptionPlan:       u.SubscriptionPlan,
		SubscriptionBuyAtMs:    u.SubscriptionBuyAtMs,
		SubscriptionExpireAtMs: u.SubscriptionExpireAtMs,
		Bio:                    u.Bio,
		Verified:               u.Verified,
	}
	if strings.TrimSpace(u.Username) != "" {
		out.Username = strings.TrimSpace(u.Username)
	}
	return json.Marshal(out)
}

type PublicProfilePage struct {
	User          User      `json:"user"`
	FollowerCount int       `json:"followerCount"`
	ActivePosts   []Auction `json:"activePosts"`
}

type ProfileUpdateInput struct {
	UserID       string  `json:"userId"`
	Name         *string `json:"name"`
	City         string  `json:"city"`
	AvatarKey    string  `json:"avatarKey"`
	ReceiveCalls *bool   `json:"receiveCalls"`
	Bio          *string `json:"bio"`
}

type FollowUserSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Phone     string `json:"phone"`
	AvatarKey string `json:"avatarKey"`
}

type BlockUserSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarKey string `json:"avatarKey"`
}

type BlockStatus struct {
	BlockedByViewer bool `json:"blockedByViewer"`
	BlockedByPeer   bool `json:"blockedByPeer"`
	CanChat         bool `json:"canChat"`
}
