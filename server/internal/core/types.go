package core

import "time"

type Site struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	GroupName  string    `json:"groupName"`
	Icon       string    `json:"icon"`
	ClickCount int64     `json:"clickCount"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Share struct {
	ID         int64      `json:"id"`
	SiteID     int64      `json:"siteId"`
	SiteName   string     `json:"siteName"`
	TargetURL  string     `json:"targetUrl"`
	Token      string     `json:"token"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	StoppedAt  *time.Time `json:"stoppedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	AccessHits int64      `json:"accessHits"`
}

type ShareWithPassword struct {
	Share         Share  `json:"share"`
	ShareURL      string `json:"shareUrl"`
	PlainPassword string `json:"plainPassword"`
}
