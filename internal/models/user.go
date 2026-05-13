package models

import "time"

type User struct {
	ID        int       `json:"id"`
	FullName  string    `json:"full_name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type SystemSetting struct {
	Key   string
	Value string
}

type AccessLog struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Action    string `json:"action"`
	CreatedAt string `json:"created_at"`
}
