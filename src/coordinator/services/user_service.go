// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	"github.com/sipcapture/homer-core/src/passwordhash"
)

// User represents a user in the system
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name,omitempty"` // display name (DB: full_name)
	PasswordHash string    `json:"-"`
	Email        string    `json:"email,omitempty"`
	IsAdmin      bool      `json:"admin"`
	IsActive     bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserService handles user management through FlightSQL
type UserService struct {
	db *sql.DB
}

// UserListFilters represents filter and sort options for user listing
type UserListFilters struct {
	Search   string
	Username string
	Email    string
	IsAdmin  *bool
	Enabled  *bool
	Sort     string
	Limit    int
}

// NewUserService creates a new user service
func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

// Authenticate validates username and password against the users table only.
func (s *UserService) Authenticate(ctx context.Context, username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	user, err := s.GetUserByUsername(ctx, username)
	if err != nil || user == nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	if !s.verifyPassword(password, user.PasswordHash) {
		return nil, fmt.Errorf("invalid credentials")
	}
	if !user.IsActive {
		return nil, fmt.Errorf("user is disabled")
	}
	return user, nil
}

// GetUserByUsername retrieves a user by username from DuckDB
func (s *UserService) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	username = strings.TrimSpace(username)

	// Case-insensitive match: legacy rows may use different casing than the UI.
	const query = `
		SELECT id, username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at
		FROM users
		WHERE lower(trim(username)) = lower(trim(?))
		LIMIT 1`
	row := s.db.QueryRowContext(ctx, query, username)
	return scanUserRow(row)
}

// GetUserByEmail retrieves a user by email (case-insensitive). Returns nil, error if not found.
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, fmt.Errorf("user not found")
	}
	const query = `
		SELECT id, username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at
		FROM users
		WHERE lower(trim(email)) = lower(trim(?))
		LIMIT 1`
	row := s.db.QueryRowContext(ctx, query, email)
	return scanUserRow(row)
}

// GetUserByID retrieves a user by ID
func (s *UserService) GetUserByID(ctx context.Context, id int64) (*User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}

	query := fmt.Sprintf(`
		SELECT id, username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at 
		FROM users 
		WHERE id = %d 
		LIMIT 1
	`, id)
	row := s.db.QueryRowContext(ctx, query)
	return scanUserRow(row)
}

// ListUsersFiltered returns users with filtering and sorting
func (s *UserService) ListUsersFiltered(ctx context.Context, filters UserListFilters) ([]*User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}

	where := make([]string, 0)

	if filters.Search != "" {
		value := "%" + sqlvalidator.SafeString(filters.Search) + "%"
		where = append(where, fmt.Sprintf("(username ILIKE '%s' OR email ILIKE '%s' OR full_name ILIKE '%s')", value, value, value))
	}
	if filters.Username != "" {
		value := "%" + sqlvalidator.SafeString(filters.Username) + "%"
		where = append(where, fmt.Sprintf("username ILIKE '%s'", value))
	}
	if filters.Email != "" {
		value := "%" + sqlvalidator.SafeString(filters.Email) + "%"
		where = append(where, fmt.Sprintf("email ILIKE '%s'", value))
	}
	if filters.IsAdmin != nil {
		where = append(where, fmt.Sprintf("is_admin = %t", *filters.IsAdmin))
	}
	if filters.Enabled != nil {
		where = append(where, fmt.Sprintf("is_active = %t", *filters.Enabled))
	}

	orderBy := "username ASC"
	if filters.Sort != "" {
		if err := sqlvalidator.ValidateExpression(filters.Sort, sqlvalidator.ExprOrderBy); err != nil {
			return nil, fmt.Errorf("invalid sort expression: %w", err)
		}
		orderBy = filters.Sort
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT id, username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at FROM users`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY " + orderBy
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]*User, 0)
	for rows.Next() {
		user, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// CreateUser inserts a new user and returns the new ID
func (s *UserService) CreateUser(ctx context.Context, username, password, email, displayName string, isAdmin, isActive bool) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("settings db not available")
	}

	passwordHash, err := passwordhash.Hash(password)
	if err != nil {
		return 0, err
	}

	query := fmt.Sprintf(
		`INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		 VALUES ('%s', '%s', '%s', '%s', %t, %t, current_timestamp, current_timestamp)
		 RETURNING id`,
		sqlvalidator.SafeString(username),
		sqlvalidator.SafeString(passwordHash),
		sqlvalidator.SafeString(email),
		sqlvalidator.SafeString(displayName),
		isAdmin,
		isActive,
	)

	var id int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateUser updates user fields and returns true if updated
func (s *UserService) UpdateUser(ctx context.Context, id int64, email, password, displayName *string, isAdmin, isActive *bool) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}

	set := make([]string, 0)
	if email != nil {
		set = append(set, fmt.Sprintf("email = '%s'", sqlvalidator.SafeString(*email)))
	}
	if password != nil {
		passwordHash, err := passwordhash.Hash(*password)
		if err != nil {
			return false, err
		}
		set = append(set, fmt.Sprintf("password_hash = '%s'", sqlvalidator.SafeString(passwordHash)))
	}
	if displayName != nil {
		set = append(set, fmt.Sprintf("full_name = '%s'", sqlvalidator.SafeString(*displayName)))
	}
	if isAdmin != nil {
		set = append(set, fmt.Sprintf("is_admin = %t", *isAdmin))
	}
	if isActive != nil {
		set = append(set, fmt.Sprintf("is_active = %t", *isActive))
	}
	if len(set) == 0 {
		return false, fmt.Errorf("no fields to update")
	}
	set = append(set, "updated_at = current_timestamp")

	query := fmt.Sprintf(
		`UPDATE users SET %s WHERE id = %d RETURNING id`,
		strings.Join(set, ", "),
		id,
	)

	var updatedID int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&updatedID); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteUser deletes a user and returns true if deleted
func (s *UserService) DeleteUser(ctx context.Context, id int64) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}

	query := fmt.Sprintf(`DELETE FROM users WHERE id = %d RETURNING id`, id)
	var deletedID int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&deletedID); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// verifyPassword checks if password matches hash (bcrypt or legacy SHA-256 hex).
func (s *UserService) verifyPassword(password, hash string) bool {
	return passwordhash.Verify(password, hash)
}

// scanUserRow converts a single row to User struct.
func scanUserRow(row *sql.Row) (*User, error) {
	var (
		user      User
		email     sql.NullString
		fullName  sql.NullString
		password  sql.NullString
		isAdmin   sql.NullBool
		isActive  sql.NullBool
		createdAt sql.NullTime
		updatedAt sql.NullTime
	)
	if err := row.Scan(&user.ID, &user.Username, &password, &email, &fullName, &isAdmin, &isActive, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	if password.Valid {
		user.PasswordHash = password.String
	}
	if email.Valid {
		user.Email = email.String
	}
	if fullName.Valid {
		user.Name = fullName.String
	}
	if isAdmin.Valid {
		user.IsAdmin = isAdmin.Bool
	}
	// NULL is_active is treated as active (legacy rows / DuckDB nullable without DEFAULT).
	if isActive.Valid {
		user.IsActive = isActive.Bool
	} else {
		user.IsActive = true
	}
	if createdAt.Valid {
		user.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		user.UpdatedAt = updatedAt.Time
	}
	return &user, nil
}

func scanUserRows(rows *sql.Rows) (*User, error) {
	var (
		user      User
		email     sql.NullString
		fullName  sql.NullString
		password  sql.NullString
		isAdmin   sql.NullBool
		isActive  sql.NullBool
		createdAt sql.NullTime
		updatedAt sql.NullTime
	)
	if err := rows.Scan(&user.ID, &user.Username, &password, &email, &fullName, &isAdmin, &isActive, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if password.Valid {
		user.PasswordHash = password.String
	}
	if email.Valid {
		user.Email = email.String
	}
	if fullName.Valid {
		user.Name = fullName.String
	}
	if isAdmin.Valid {
		user.IsAdmin = isAdmin.Bool
	}
	// NULL is_active is treated as active (legacy rows / DuckDB nullable without DEFAULT).
	if isActive.Valid {
		user.IsActive = isActive.Bool
	} else {
		user.IsActive = true
	}
	if createdAt.Valid {
		user.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		user.UpdatedAt = updatedAt.Time
	}
	return &user, nil
}

// escapeSQL escapes a string for SQL.
// Deprecated: use sqlvalidator.SafeString instead.
func escapeSQL(s string) string {
	return sqlvalidator.SafeString(s)
}

// escapeJSONData escapes a JSON/script blob for embedding in a DuckDB
// single-quoted SQL string. Unlike escapeSQL/SafeString it does NOT truncate,
// because fields_mapping and Lua scripts can exceed the SafeString length limit.
//
// DuckDB does not treat backslash as an escape inside '...' literals (only ''
// doubles a quote). GHSA-vxc3-v6h9-8856 recommended also doubling backslashes
// to match SafeString; that would store extra backslashes and corrupt JSON \n,
// \t, \uXXXX and Windows paths. Do not apply that change unless the parser
// starts interpreting \ as an escape. Prefer bound parameters (?) for new code.
func escapeJSONData(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	for _, r := range s {
		if r == 0 {
			continue
		}
		if r == '\'' {
			b.WriteString("''")
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
