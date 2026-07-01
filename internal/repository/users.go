package repository

import (
	"context"
	"database/sql"
	"fmt"

	"map-interactif/internal/models"
)

// ListUsers returns all non-deleted users.
func ListUsers(ctx context.Context, db *sql.DB) ([]models.User, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, full_name, email, role, status, last_login_at, created_at
		FROM users
		WHERE COALESCE(status, 'active') != 'deleted'
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := []models.User{}
	for rows.Next() {
		var u models.User
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.ID, &u.FullName, &u.Email, &u.Role, &u.Status, &lastLogin, &u.CreatedAt); err != nil {
			return nil, err
		}
		if lastLogin.Valid {
			u.LastLoginAt = &lastLogin.Time
		}
		res = append(res, u)
	}
	return res, nil
}

// GetUserByID fetches a single user by id.
func GetUserByID(ctx context.Context, db *sql.DB, id int) (*models.User, error) {
	var u models.User
	var lastLogin sql.NullTime
	err := db.QueryRowContext(ctx, `
		SELECT id, full_name, email, role, status, last_login_at, created_at
		FROM users WHERE id=$1`, id).Scan(
		&u.ID, &u.FullName, &u.Email, &u.Role, &u.Status, &lastLogin, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return &u, nil
}

// EmailExists returns true if a user with this email exists (excluding the given id).
func EmailExists(ctx context.Context, db *sql.DB, email string, excludeID int) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email=$1 AND id != $2 AND status != 'deleted')`,
		email, excludeID).Scan(&exists)
	return exists, err
}

// InsertUser inserts a new user and returns the new ID.
// u.PasswordHash must already be a bcrypt hash if provided.
func InsertUser(ctx context.Context, db *sql.DB, u models.User) (int, error) {
	var id int
	var err error
	if u.PasswordHash != "" {
		err = db.QueryRowContext(ctx, `
			INSERT INTO users (full_name, email, password_hash, role, status)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			u.FullName, u.Email, u.PasswordHash, u.Role, u.Status).Scan(&id)
	} else {
		err = db.QueryRowContext(ctx, `
			INSERT INTO users (full_name, email, role, status)
			VALUES ($1, $2, $3, $4) RETURNING id`,
			u.FullName, u.Email, u.Role, u.Status).Scan(&id)
	}
	return id, err
}

// UpdateUser updates editable user fields.
func UpdateUser(ctx context.Context, db *sql.DB, id int, fullName, email, role, status string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE users
		SET full_name=$1, email=$2, role=$3, status=$4, updated_at=NOW()
		WHERE id=$5`, fullName, email, role, status, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("utilisateur introuvable")
	}
	return nil
}

// SoftDeleteUser marks a user as deleted (preserves audit history).
func SoftDeleteUser(ctx context.Context, db *sql.DB, id int) error {
	res, err := db.ExecContext(ctx,
		`UPDATE users SET status='deleted', updated_at=NOW() WHERE id=$1 AND status != 'deleted'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("utilisateur introuvable ou déjà supprimé")
	}
	return nil
}
