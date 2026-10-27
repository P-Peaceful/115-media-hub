package model

import "time"

// The credential is plaintext; protect the database and every backup as login material.
type Connection struct {
	ID                string     `gorm:"column:id;primaryKey"`
	Kind              string     `gorm:"column:kind"`
	Name              string     `gorm:"column:name"`
	ExternalAccountID string     `gorm:"column:external_account_id"`
	BaseURL           string     `gorm:"column:base_url"`
	CredentialJSON    string     `gorm:"column:credential_json"`
	CapabilitiesJSON  string     `gorm:"column:capabilities_json"`
	ProfileJSON       string     `gorm:"column:profile_json"`
	IsActive          bool       `gorm:"column:is_active"`
	Status            string     `gorm:"column:status"`
	RetryAfterAt      *time.Time `gorm:"column:retry_after_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (Connection) TableName() string { return "connections" }
