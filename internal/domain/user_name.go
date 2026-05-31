package domain

import "strings"

// ResolveName is the public label: custom display name if set, otherwise the account id.
func ResolveName(userID string, customName string) string {
	if n := strings.TrimSpace(customName); n != "" {
		return n
	}
	if id := strings.TrimSpace(userID); id != "" {
		return id
	}
	return "کاربر"
}

func (u User) resolvedName() string {
	return ResolveName(u.ID, u.CustomName)
}
