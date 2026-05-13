package repository

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// OpenDB opens a connection to the PostgreSQL database.
func OpenDB() (*sql.DB, error) {
	dsn, err := buildDSN()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		if !isMissingDatabaseError(err) {
			return nil, err
		}

		if err := createDatabase(); err != nil {
			return nil, err
		}

		if err := db.Ping(); err != nil {
			return nil, err
		}
	}

	return db, nil
}

func isMissingDatabaseError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	if strings.Contains(message, "SQLSTATE 3D000") {
		return true
	}

	return strings.Contains(message, "database") && strings.Contains(message, "does not exist")
}

func createDatabase() error {
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslmode := strings.TrimSpace(os.Getenv("DB_SSLMODE"))

	if host == "" || user == "" || name == "" {
		return fmt.Errorf("DB_HOST, DB_USER, and DB_NAME must be set to create the database")
	}

	if port == "" {
		port = "5432"
	}
	if sslmode == "" {
		sslmode = "disable"
	}

	adminURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   "postgres",
	}

	query := adminURL.Query()
	query.Set("sslmode", sslmode)
	adminURL.RawQuery = query.Encode()

	adminDB, err := sql.Open("pgx", adminURL.String())
	if err != nil {
		return err
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return err
	}

	_, err = adminDB.Exec("CREATE DATABASE " + quoteIdentifier(name))
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return err
	}

	return nil
}

func quoteIdentifier(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\""
}

func buildDSN() (string, error) {
	if value := strings.TrimSpace(os.Getenv("DATABASE_URL")); value != "" {
		return value, nil
	}

	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslmode := strings.TrimSpace(os.Getenv("DB_SSLMODE"))

	if host == "" || user == "" || name == "" {
		return "", fmt.Errorf("DATABASE_URL or DB_HOST/DB_USER/DB_NAME must be set")
	}

	if port == "" {
		port = "5432"
	}
	if sslmode == "" {
		sslmode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   name,
	}

	q := u.Query()
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()

	return u.String(), nil
}
