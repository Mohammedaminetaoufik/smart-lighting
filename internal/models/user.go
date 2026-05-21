package models

import "time"

type User struct {
	ID          int        `json:"id"`
	FullName    string     `json:"full_name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type SystemSetting struct {
	Key   string
	Value string
}

type AccessLog struct {
	ID         int                    `json:"id"`
	UserID     *int                   `json:"user_id,omitempty"`
	UserName   string                 `json:"user_name,omitempty"`
	Action     string                 `json:"action"`
	TargetType string                 `json:"target_type,omitempty"`
	TargetID   *int                   `json:"target_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	IPAddress  string                 `json:"ip_address,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	CreatedAt  string                 `json:"created_at"`
}

type AuditLog struct {
	ID              int                    `json:"id"`
	UserID          *int                   `json:"user_id,omitempty"`
	UserName        string                 `json:"user_name,omitempty"`
	UserRole        string                 `json:"user_role,omitempty"`
	Action          string                 `json:"action"`
	EntityType      string                 `json:"entity_type"`
	EntityID        *int                   `json:"entity_id,omitempty"`
	EntityReference string                 `json:"entity_reference,omitempty"`
	Description     string                 `json:"description"`
	OldValues       map[string]interface{} `json:"old_values,omitempty"`
	NewValues       map[string]interface{} `json:"new_values,omitempty"`
	Status          string                 `json:"status"`
	IPAddress       string                 `json:"ip_address,omitempty"`
	UserAgent       string                 `json:"user_agent,omitempty"`
	CreatedAt       string                 `json:"created_at"`
}
