package main

import (
"context"
"database/sql"
)

func listUsers(ctx context.Context, db *sql.DB) ([]User, error) {
rows, err := db.QueryContext(ctx, "SELECT id, full_name, email, role, status, created_at FROM users ORDER BY id")
if err != nil {
return nil, err
}
defer rows.Close()
var res []User
for rows.Next() {
var u User
if err := rows.Scan(&u.ID, &u.FullName, &u.Email, &u.Role, &u.Status, &u.CreatedAt); err != nil {
return nil, err
}
res = append(res, u)
}
return res, nil
}

func insertUser(ctx context.Context, db *sql.DB, u User) (int, error) {
var id int
err := db.QueryRowContext(ctx, "INSERT INTO users (full_name, email, role, status) VALUES (,,,) RETURNING id", u.FullName, u.Email, u.Role, u.Status).Scan(&id)
return id, err
}
