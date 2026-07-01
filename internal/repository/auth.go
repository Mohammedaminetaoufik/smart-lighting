package repository

import (
	"context"
	"database/sql"
	"fmt"

	"map-interactif/internal/models"
)

// FindUserByEmail retrieves a user including password_hash for authentication.
func FindUserByEmail(ctx context.Context, db *sql.DB, email string) (*models.User, error) {
	var u models.User
	var lastLogin sql.NullTime
	err := db.QueryRowContext(ctx, `
		SELECT id, full_name, email, COALESCE(password_hash, ''), role, status, last_login_at, created_at
		FROM users WHERE email=$1 AND status != 'deleted'`, email).Scan(
		&u.ID, &u.FullName, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &lastLogin, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("utilisateur introuvable")
	}
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return &u, nil
}

// UpdateLastLogin updates the last_login_at timestamp for a user.
func UpdateLastLogin(ctx context.Context, db *sql.DB, id int) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET last_login_at=NOW(), updated_at=NOW() WHERE id=$1`, id)
	return err
}

// UpdatePassword sets a new bcrypt password hash for a user.
func UpdatePassword(ctx context.Context, db *sql.DB, id int, hash string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, hash, id)
	return err
}
