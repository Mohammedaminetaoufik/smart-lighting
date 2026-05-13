package repository

import (
	"context"
	"database/sql"

	"map-interactif/internal/models"
)

// ListUsers returns all users.
func ListUsers(ctx context.Context, db *sql.DB) ([]models.User, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, full_name, email, role, status, created_at FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.FullName, &u.Email, &u.Role, &u.Status, &u.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, u)
	}
	return res, nil
}

// InsertUser inserts a new user and returns the new ID.
func InsertUser(ctx context.Context, db *sql.DB, u models.User) (int, error) {
	var id int
	err := db.QueryRowContext(ctx, "INSERT INTO users (full_name, email, role, status) VALUES ($1, $2, $3, $4) RETURNING id", u.FullName, u.Email, u.Role, u.Status).Scan(&id)
	return id, err
}
