package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type UserCredential struct {
	User
	PasswordHash string
}

type FXOBox struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	SIPUsername string    `json:"sip_username"`
	SIPPassword string    `json:"sip_password,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Registration struct {
	FXOBoxID   int64     `json:"fxo_box_id"`
	ContactURI string    `json:"contact_uri"`
	SourceAddr string    `json:"source_addr"`
	Transport  string    `json:"transport"`
	CallID     string    `json:"call_id"`
	UserAgent  string    `json:"user_agent"`
	ExpiresAt  time.Time `json:"expires_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type FXOBoxWithStatus struct {
	FXOBox
	Online       bool          `json:"online"`
	InUse        bool          `json:"in_use"`
	Registration *Registration `json:"registration,omitempty"`
}

type FXOBoxForUser struct {
	FXOBoxWithStatus
	CanDial        bool `json:"can_dial"`
	CanReceive     bool `json:"can_receive"`
	NotifyIncoming bool `json:"notify_incoming"`
}

type CallLog struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	FXOBoxID   int64      `json:"fxo_box_id"`
	FXOBoxName string     `json:"fxo_box_name"`
	Number     string     `json:"number"`
	StartedAt  time.Time  `json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason"`
}

type Contact struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Number    string    `json:"number"`
	CreatedAt time.Time `json:"created_at"`
}

type UserDevice struct {
	DeviceToken      string    `json:"device_token"`
	UserID           int64     `json:"user_id"`
	ClientType       string    `json:"client_type"`
	DeviceName       string    `json:"device_name"`
	PushPlatform     string    `json:"push_platform"`
	PushToken        string    `json:"push_token,omitempty"`
	RefreshTokenHash string    `json:"-"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type UserFXOPermission struct {
	UserID         int64     `json:"user_id"`
	FXOBoxID       int64     `json:"fxo_box_id"`
	CanDial        bool      `json:"can_dial"`
	CanReceive     bool      `json:"can_receive"`
	NotifyIncoming bool      `json:"notify_incoming"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}
	if err := pingWithTimeout(db); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func pingWithTimeout(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS fxo_boxes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			sip_username TEXT NOT NULL UNIQUE,
			sip_password TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS fxo_registrations (
			fxo_box_id INTEGER PRIMARY KEY,
			contact_uri TEXT NOT NULL,
			source_addr TEXT NOT NULL,
			transport TEXT NOT NULL,
			call_id TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			expires_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY(fxo_box_id) REFERENCES fxo_boxes(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS call_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 0,
			fxo_box_id INTEGER NOT NULL,
			number TEXT NOT NULL,
			started_at INTEGER NOT NULL,
			ended_at INTEGER,
			status TEXT NOT NULL,
			reason TEXT NOT NULL,
			FOREIGN KEY(fxo_box_id) REFERENCES fxo_boxes(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS contacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			number TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS user_devices (
			device_token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			client_type TEXT NOT NULL,
			device_name TEXT NOT NULL,
			push_platform TEXT NOT NULL DEFAULT '',
			push_token TEXT NOT NULL DEFAULT '',
			refresh_token_hash TEXT NOT NULL DEFAULT '',
			refresh_expires_at INTEGER NOT NULL DEFAULT 0,
			last_seen_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS user_fxo_permissions (
			user_id INTEGER NOT NULL,
			fxo_box_id INTEGER NOT NULL,
			can_dial INTEGER NOT NULL DEFAULT 0,
			can_receive INTEGER NOT NULL DEFAULT 0,
			notify_incoming INTEGER NOT NULL DEFAULT 1,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(user_id, fxo_box_id),
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY(fxo_box_id) REFERENCES fxo_boxes(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_user_id ON contacts(user_id, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_user_id_name ON contacts(user_id, name COLLATE NOCASE);`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_user_id_number ON contacts(user_id, number COLLATE NOCASE);`,
		`CREATE INDEX IF NOT EXISTS idx_user_devices_user_id ON user_devices(user_id, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_user_devices_push ON user_devices(push_token);`,
		`CREATE INDEX IF NOT EXISTS idx_user_fxo_permissions_box ON user_fxo_permissions(fxo_box_id, user_id);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate query failed: %w", err)
		}
	}
	if err := s.ensureUserRoleColumn(); err != nil {
		return err
	}
	if err := s.ensureUserFxoNotifyIncomingColumn(); err != nil {
		return err
	}
	if err := s.ensureCallLogUserColumn(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_call_logs_user_id_id ON call_logs(user_id, id DESC)`); err != nil {
		return fmt.Errorf("create call_logs user index: %w", err)
	}
	return nil
}

func (s *Store) ensureUserRoleColumn() error {
	rows, err := s.db.Query(`PRAGMA table_info(users)`)
	if err != nil {
		return fmt.Errorf("query users table_info: %w", err)
	}
	defer rows.Close()

	hasRole := false
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan users table_info: %w", err)
		}
		if strings.EqualFold(name, "role") {
			hasRole = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate users table_info: %w", err)
	}
	if !hasRole {
		if _, err := s.db.Exec(`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`); err != nil {
			return fmt.Errorf("add users.role column: %w", err)
		}
	}
	if _, err := s.db.Exec(`UPDATE users SET role = 'user' WHERE role IS NULL OR TRIM(role) = ''`); err != nil {
		return fmt.Errorf("backfill users.role: %w", err)
	}
	return nil
}

func (s *Store) ensureCallLogUserColumn() error {
	rows, err := s.db.Query(`PRAGMA table_info(call_logs)`)
	if err != nil {
		return fmt.Errorf("query call_logs table_info: %w", err)
	}
	defer rows.Close()

	hasUserID := false
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan call_logs table_info: %w", err)
		}
		if strings.EqualFold(name, "user_id") {
			hasUserID = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate call_logs table_info: %w", err)
	}
	if !hasUserID {
		if _, err := s.db.Exec(`ALTER TABLE call_logs ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add call_logs.user_id column: %w", err)
		}
	}
	return nil
}

func (s *Store) ensureUserFxoNotifyIncomingColumn() error {
	rows, err := s.db.Query(`PRAGMA table_info(user_fxo_permissions)`)
	if err != nil {
		return fmt.Errorf("query user_fxo_permissions table_info: %w", err)
	}
	defer rows.Close()

	hasNotify := false
	for rows.Next() {
		var (
			cid       int
			name      string
			typ       string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan user_fxo_permissions table_info: %w", err)
		}
		if strings.EqualFold(name, "notify_incoming") {
			hasNotify = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate user_fxo_permissions table_info: %w", err)
	}
	if !hasNotify {
		if _, err := s.db.Exec(`ALTER TABLE user_fxo_permissions ADD COLUMN notify_incoming INTEGER NOT NULL DEFAULT 1`); err != nil {
			return fmt.Errorf("add user_fxo_permissions.notify_incoming column: %w", err)
		}
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash, role string) (User, error) {
	now := time.Now().Unix()
	role = normalizeRole(role)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, role, now,
	)
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Username: username, Role: role, CreatedAt: time.Unix(now, 0)}, nil
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete user tx: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_ = tx.Rollback()
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM contacts WHERE user_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete user contacts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM call_logs WHERE user_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete user call logs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete user tx: %w", err)
	}
	return nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var c int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&c); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return c, nil
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var c int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE role = ?`, RoleAdmin).Scan(&c); err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return c, nil
}

func (s *Store) SetUserRoleByUsername(ctx context.Context, username, role string) error {
	role = normalizeRole(role)
	res, err := s.db.ExecContext(ctx, `UPDATE users SET role = ? WHERE username = ?`, role, username)
	if err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, role, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var u User
		var created int64
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &created); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.Role = normalizeRole(u.Role)
		u.CreatedAt = time.Unix(created, 0)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (s *Store) GetUserCredentialByUsername(ctx context.Context, username string) (UserCredential, error) {
	var uc UserCredential
	var created int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?`, username,
	).Scan(&uc.ID, &uc.Username, &uc.PasswordHash, &uc.Role, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserCredential{}, ErrNotFound
		}
		return UserCredential{}, fmt.Errorf("get user credential: %w", err)
	}
	uc.Role = normalizeRole(uc.Role)
	uc.CreatedAt = time.Unix(created, 0)
	return uc, nil
}

func (s *Store) GetUserCredentialByID(ctx context.Context, id int64) (UserCredential, error) {
	var uc UserCredential
	var created int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?`, id,
	).Scan(&uc.ID, &uc.Username, &uc.PasswordHash, &uc.Role, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserCredential{}, ErrNotFound
		}
		return UserCredential{}, fmt.Errorf("get user credential by id: %w", err)
	}
	uc.Role = normalizeRole(uc.Role)
	uc.CreatedAt = time.Unix(created, 0)
	return uc, nil
}

func (s *Store) UpdateUserPasswordHashByID(ctx context.Context, id int64, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("update user password hash: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get app setting: %w", err)
	}
	return value, nil
}

func (s *Store) UpsertSetting(ctx context.Context, key, value string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, key, value, now)
	if err != nil {
		return fmt.Errorf("upsert app setting: %w", err)
	}
	return nil
}

func (s *Store) CreateFXOBox(ctx context.Context, name, sipUsername, sipPassword string) (FXOBox, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO fxo_boxes (name, sip_username, sip_password, created_at) VALUES (?, ?, ?, ?)`,
		name, sipUsername, sipPassword, now,
	)
	if err != nil {
		return FXOBox{}, fmt.Errorf("insert fxo box: %w", err)
	}
	id, _ := res.LastInsertId()
	return FXOBox{
		ID:          id,
		Name:        name,
		SIPUsername: sipUsername,
		SIPPassword: sipPassword,
		CreatedAt:   time.Unix(now, 0),
	}, nil
}

func (s *Store) UpdateFXOBox(ctx context.Context, id int64, name, sipUsername, sipPassword string) (FXOBox, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE fxo_boxes SET name = ?, sip_username = ?, sip_password = ? WHERE id = ?`,
		name, sipUsername, sipPassword, id,
	)
	if err != nil {
		return FXOBox{}, fmt.Errorf("update fxo box: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return FXOBox{}, ErrNotFound
	}
	return s.GetFXOBoxByID(ctx, id)
}

func (s *Store) DeleteFXOBox(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM fxo_boxes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete fxo box: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetFXOBoxByID(ctx context.Context, id int64) (FXOBox, error) {
	var b FXOBox
	var created int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, sip_username, sip_password, created_at FROM fxo_boxes WHERE id = ?`, id,
	).Scan(&b.ID, &b.Name, &b.SIPUsername, &b.SIPPassword, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FXOBox{}, ErrNotFound
		}
		return FXOBox{}, fmt.Errorf("get fxo by id: %w", err)
	}
	b.CreatedAt = time.Unix(created, 0)
	return b, nil
}

func (s *Store) GetFXOBoxBySIPUsername(ctx context.Context, username string) (FXOBox, error) {
	var b FXOBox
	var created int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, sip_username, sip_password, created_at FROM fxo_boxes WHERE sip_username = ?`, username,
	).Scan(&b.ID, &b.Name, &b.SIPUsername, &b.SIPPassword, &created)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FXOBox{}, ErrNotFound
		}
		return FXOBox{}, fmt.Errorf("get fxo by username: %w", err)
	}
	b.CreatedAt = time.Unix(created, 0)
	return b, nil
}

func (s *Store) ListFXOBoxesWithStatus(ctx context.Context) ([]FXOBoxWithStatus, error) {
	return s.listFXOBoxesWithStatus(ctx, `
		SELECT
			b.id, b.name, b.sip_username, b.sip_password, b.created_at,
			r.contact_uri, r.source_addr, r.transport, r.call_id, r.user_agent,
			r.expires_at, r.updated_at
		FROM fxo_boxes b
		LEFT JOIN fxo_registrations r ON r.fxo_box_id = b.id
		ORDER BY b.id ASC`)
}

func (s *Store) ListFXOBoxesWithStatusForUser(ctx context.Context, userID int64, includeAll bool) ([]FXOBoxForUser, error) {
	query := `
		SELECT
			b.id, b.name, b.sip_username, b.sip_password, b.created_at,
			r.contact_uri, r.source_addr, r.transport, r.call_id, r.user_agent,
			r.expires_at, r.updated_at,
			p.can_dial, p.can_receive, p.notify_incoming
		FROM fxo_boxes b
		LEFT JOIN user_fxo_permissions p ON p.fxo_box_id = b.id AND p.user_id = ?
		LEFT JOIN fxo_registrations r ON r.fxo_box_id = b.id
	`
	if !includeAll {
		query += ` WHERE p.can_dial = 1`
	}
	query += ` ORDER BY b.id ASC`

	now := time.Now().Unix()
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list fxo boxes for user: %w", err)
	}
	defer rows.Close()

	out := make([]FXOBoxForUser, 0)
	for rows.Next() {
		var item FXOBoxForUser
		var rContactURI, rSourceAddr, rTransport, rCallID, rUserAgent sql.NullString
		var rExpiresAt, rUpdatedAt sql.NullInt64
		var created int64
		var canDial, canReceive, notifyIncoming sql.NullInt64

		if err := rows.Scan(
			&item.ID, &item.Name, &item.SIPUsername, &item.SIPPassword, &created,
			&rContactURI, &rSourceAddr, &rTransport, &rCallID, &rUserAgent,
			&rExpiresAt, &rUpdatedAt,
			&canDial, &canReceive, &notifyIncoming,
		); err != nil {
			return nil, fmt.Errorf("scan fxo box for user: %w", err)
		}
		item.CreatedAt = time.Unix(created, 0)
		if includeAll {
			item.CanDial = true
			item.CanReceive = true
			// for notify_incoming, we default to 1 if null, else use the explicit preference
			if notifyIncoming.Valid {
				item.NotifyIncoming = notifyIncoming.Int64 != 0
			} else {
				item.NotifyIncoming = true
			}
		} else {
			item.CanDial = canDial.Valid && canDial.Int64 != 0
			item.CanReceive = canReceive.Valid && canReceive.Int64 != 0
			item.NotifyIncoming = notifyIncoming.Valid && notifyIncoming.Int64 != 0
		}

		if rContactURI.Valid {
			item.Registration = &Registration{
				FXOBoxID:   item.ID,
				ContactURI: rContactURI.String,
				SourceAddr: rSourceAddr.String,
				Transport:  rTransport.String,
				CallID:     rCallID.String,
				UserAgent:  rUserAgent.String,
				ExpiresAt:  time.Unix(rExpiresAt.Int64, 0),
				UpdatedAt:  time.Unix(rUpdatedAt.Int64, 0),
			}
			item.Online = item.Registration.ExpiresAt.After(time.Unix(now, 0))
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fxo boxes for user: %w", err)
	}
	return out, nil
}

func (s *Store) listFXOBoxesWithStatus(ctx context.Context, query string, args ...any) ([]FXOBoxWithStatus, error) {
	now := time.Now().Unix()
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list fxo boxes: %w", err)
	}
	defer rows.Close()

	out := make([]FXOBoxWithStatus, 0)
	for rows.Next() {
		var item FXOBoxWithStatus
		var created int64
		var contact, source, transport, callID, userAgent sql.NullString
		var expiresAt, updatedAt sql.NullInt64

		if err := rows.Scan(
			&item.ID, &item.Name, &item.SIPUsername, &item.SIPPassword, &created,
			&contact, &source, &transport, &callID, &userAgent,
			&expiresAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan fxo row: %w", err)
		}
		item.CreatedAt = time.Unix(created, 0)

		if expiresAt.Valid {
			reg := &Registration{
				FXOBoxID:   item.ID,
				ContactURI: contact.String,
				SourceAddr: source.String,
				Transport:  transport.String,
				CallID:     callID.String,
				UserAgent:  userAgent.String,
				ExpiresAt:  time.Unix(expiresAt.Int64, 0),
				UpdatedAt:  time.Unix(updatedAt.Int64, 0),
			}
			item.Registration = reg
			item.Online = expiresAt.Int64 > now
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fxo rows: %w", err)
	}
	return out, nil
}

func (s *Store) UpsertRegistration(ctx context.Context, reg Registration) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO fxo_registrations
			(fxo_box_id, contact_uri, source_addr, transport, call_id, user_agent, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(fxo_box_id) DO UPDATE SET
			contact_uri = excluded.contact_uri,
			source_addr = excluded.source_addr,
			transport = excluded.transport,
			call_id = excluded.call_id,
			user_agent = excluded.user_agent,
			expires_at = excluded.expires_at,
			updated_at = excluded.updated_at
	`, reg.FXOBoxID, reg.ContactURI, reg.SourceAddr, reg.Transport, reg.CallID, reg.UserAgent, reg.ExpiresAt.Unix(), reg.UpdatedAt.Unix())
	if err != nil {
		return fmt.Errorf("upsert registration: %w", err)
	}
	return nil
}

func (s *Store) DeleteRegistration(ctx context.Context, boxID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM fxo_registrations WHERE fxo_box_id = ?`, boxID)
	if err != nil {
		return fmt.Errorf("delete registration: %w", err)
	}
	return nil
}

func (s *Store) GetActiveRegistration(ctx context.Context, boxID int64) (Registration, error) {
	now := time.Now().Unix()
	var r Registration
	var expires, updated int64
	err := s.db.QueryRowContext(ctx, `
		SELECT fxo_box_id, contact_uri, source_addr, transport, call_id, user_agent, expires_at, updated_at
		FROM fxo_registrations
		WHERE fxo_box_id = ? AND expires_at > ?`, boxID, now,
	).Scan(&r.FXOBoxID, &r.ContactURI, &r.SourceAddr, &r.Transport, &r.CallID, &r.UserAgent, &expires, &updated)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Registration{}, ErrNotFound
		}
		return Registration{}, fmt.Errorf("get active registration: %w", err)
	}
	r.ExpiresAt = time.Unix(expires, 0)
	r.UpdatedAt = time.Unix(updated, 0)
	return r, nil
}

func (s *Store) CleanupExpiredRegistrations(ctx context.Context) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM fxo_registrations WHERE expires_at <= ?`, now)
	if err != nil {
		return fmt.Errorf("cleanup registrations: %w", err)
	}
	return nil
}

func (s *Store) CreateCallLog(ctx context.Context, userID, boxID int64, number, status, reason string) (int64, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO call_logs (user_id, fxo_box_id, number, started_at, status, reason) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, boxID, number, now, status, reason,
	)
	if err != nil {
		return 0, fmt.Errorf("insert call log: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *Store) EndCallLog(ctx context.Context, id int64, status, reason string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx,
		`UPDATE call_logs SET ended_at = ?, status = ?, reason = ? WHERE id = ?`, now, status, reason, id,
	)
	if err != nil {
		return fmt.Errorf("end call log: %w", err)
	}
	return nil
}

func (s *Store) ListCallLogsByUser(ctx context.Context, userID int64, page, pageSize int) ([]CallLog, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM call_logs WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count call logs: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			l.id, l.user_id, l.fxo_box_id, COALESCE(b.name, ''), l.number,
			l.started_at, l.ended_at, l.status, l.reason
		FROM call_logs l
		LEFT JOIN fxo_boxes b ON b.id = l.fxo_box_id
		WHERE l.user_id = ?
		ORDER BY l.id DESC
		LIMIT ? OFFSET ?`, userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list call logs: %w", err)
	}
	defer rows.Close()

	out := make([]CallLog, 0, pageSize)
	for rows.Next() {
		var item CallLog
		var started int64
		var ended sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.FXOBoxID, &item.FXOBoxName, &item.Number,
			&started, &ended, &item.Status, &item.Reason,
		); err != nil {
			return nil, 0, fmt.Errorf("scan call log: %w", err)
		}
		item.StartedAt = time.Unix(started, 0)
		if ended.Valid {
			tm := time.Unix(ended.Int64, 0)
			item.EndedAt = &tm
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate call logs: %w", err)
	}
	return out, total, nil
}

func (s *Store) CreateContact(ctx context.Context, userID int64, name, number string) (Contact, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO contacts (user_id, name, number, created_at) VALUES (?, ?, ?, ?)`,
		userID, name, number, now,
	)
	if err != nil {
		return Contact{}, fmt.Errorf("insert contact: %w", err)
	}
	id, _ := res.LastInsertId()
	return Contact{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Number:    number,
		CreatedAt: time.Unix(now, 0),
	}, nil
}

func (s *Store) DeleteContact(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM contacts WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete contact: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListContacts(ctx context.Context, userID int64, q string, limit int) ([]Contact, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	q = strings.TrimSpace(q)

	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, user_id, name, number, created_at
			FROM contacts
			WHERE user_id = ?
			ORDER BY id DESC
			LIMIT ?`, userID, limit)
	} else {
		pattern := "%" + q + "%"
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, user_id, name, number, created_at
			FROM contacts
			WHERE user_id = ? AND (name LIKE ? COLLATE NOCASE OR number LIKE ? COLLATE NOCASE)
			ORDER BY id DESC
			LIMIT ?`, userID, pattern, pattern, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	out := make([]Contact, 0)
	for rows.Next() {
		var item Contact
		var created int64
		if err := rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Number, &created); err != nil {
			return nil, fmt.Errorf("scan contact: %w", err)
		}
		item.CreatedAt = time.Unix(created, 0)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", err)
	}
	return out, nil
}

func (s *Store) UpsertUserDevice(ctx context.Context, dev UserDevice) error {
	now := time.Now()
	createdAt := now.Unix()
	if !dev.CreatedAt.IsZero() {
		createdAt = dev.CreatedAt.Unix()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_devices
			(device_token, user_id, client_type, device_name, push_platform, push_token, refresh_token_hash, refresh_expires_at, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_token) DO UPDATE SET
			user_id = excluded.user_id,
			client_type = excluded.client_type,
			device_name = excluded.device_name,
			push_platform = CASE WHEN excluded.push_platform <> '' THEN excluded.push_platform ELSE user_devices.push_platform END,
			push_token = CASE WHEN excluded.push_token <> '' THEN excluded.push_token ELSE user_devices.push_token END,
			refresh_token_hash = excluded.refresh_token_hash,
			refresh_expires_at = excluded.refresh_expires_at,
			last_seen_at = excluded.last_seen_at,
			updated_at = excluded.updated_at
	`, strings.TrimSpace(dev.DeviceToken), dev.UserID, strings.TrimSpace(dev.ClientType), strings.TrimSpace(dev.DeviceName),
		strings.TrimSpace(dev.PushPlatform), strings.TrimSpace(dev.PushToken), strings.TrimSpace(dev.RefreshTokenHash),
		dev.RefreshExpiresAt.Unix(), now.Unix(), createdAt, now.Unix())
	if err != nil {
		return fmt.Errorf("upsert user device: %w", err)
	}
	return nil
}

func (s *Store) GetUserDeviceByToken(ctx context.Context, deviceToken string) (UserDevice, error) {
	var dev UserDevice
	var refreshExpiresAt, lastSeenAt, createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `
		SELECT device_token, user_id, client_type, device_name, push_platform, push_token, refresh_token_hash,
			refresh_expires_at, last_seen_at, created_at, updated_at
		FROM user_devices
		WHERE device_token = ?`, strings.TrimSpace(deviceToken),
	).Scan(&dev.DeviceToken, &dev.UserID, &dev.ClientType, &dev.DeviceName, &dev.PushPlatform, &dev.PushToken,
		&dev.RefreshTokenHash, &refreshExpiresAt, &lastSeenAt, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserDevice{}, ErrNotFound
		}
		return UserDevice{}, fmt.Errorf("get user device: %w", err)
	}
	dev.RefreshExpiresAt = time.Unix(refreshExpiresAt, 0)
	dev.LastSeenAt = time.Unix(lastSeenAt, 0)
	dev.CreatedAt = time.Unix(createdAt, 0)
	dev.UpdatedAt = time.Unix(updatedAt, 0)
	return dev, nil
}

func (s *Store) TouchUserDevice(ctx context.Context, userID int64, deviceToken string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE user_devices
		SET last_seen_at = ?, updated_at = ?
		WHERE user_id = ? AND device_token = ?`, now, now, userID, strings.TrimSpace(deviceToken))
	if err != nil {
		return fmt.Errorf("touch user device: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateUserDevicePush(ctx context.Context, userID int64, deviceToken, pushPlatform, pushToken string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE user_devices
		SET push_platform = ?, push_token = ?, last_seen_at = ?, updated_at = ?
		WHERE user_id = ? AND device_token = ?`,
		strings.TrimSpace(pushPlatform), strings.TrimSpace(pushToken), now, now, userID, strings.TrimSpace(deviceToken))
	if err != nil {
		return fmt.Errorf("update user device push: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearUserDeviceAuth(ctx context.Context, userID int64, deviceToken string) error {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		UPDATE user_devices
		SET refresh_token_hash = '', refresh_expires_at = 0, updated_at = ?
		WHERE user_id = ? AND device_token = ?`, now, userID, strings.TrimSpace(deviceToken))
	if err != nil {
		return fmt.Errorf("clear user device auth: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearUserDeviceAuthByUserID(ctx context.Context, userID int64) error {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE user_devices
		SET refresh_token_hash = '', refresh_expires_at = 0, updated_at = ?
		WHERE user_id = ?`, now, userID); err != nil {
		return fmt.Errorf("clear user auth by user id: %w", err)
	}
	return nil
}

func (s *Store) ListUserDevicesByUserIDs(ctx context.Context, userIDs []int64) ([]UserDevice, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(userIDs))
	args := make([]any, 0, len(userIDs))
	for _, id := range userIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT device_token, user_id, client_type, device_name, push_platform, push_token, refresh_token_hash,
			refresh_expires_at, last_seen_at, created_at, updated_at
		FROM user_devices
		WHERE user_id IN (%s)`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("list user devices: %w", err)
	}
	defer rows.Close()

	out := make([]UserDevice, 0)
	for rows.Next() {
		var dev UserDevice
		var refreshExpiresAt, lastSeenAt, createdAt, updatedAt int64
		if err := rows.Scan(&dev.DeviceToken, &dev.UserID, &dev.ClientType, &dev.DeviceName, &dev.PushPlatform, &dev.PushToken,
			&dev.RefreshTokenHash, &refreshExpiresAt, &lastSeenAt, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan user device: %w", err)
		}
		dev.RefreshExpiresAt = time.Unix(refreshExpiresAt, 0)
		dev.LastSeenAt = time.Unix(lastSeenAt, 0)
		dev.CreatedAt = time.Unix(createdAt, 0)
		dev.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, dev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user devices: %w", err)
	}
	return out, nil
}

func (s *Store) SetUserFXOPermission(ctx context.Context, userID, boxID int64, canDial, canReceive bool) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_fxo_permissions (user_id, fxo_box_id, can_dial, can_receive, notify_incoming, updated_at)
		VALUES (?, ?, ?, ?, 1, ?)
		ON CONFLICT(user_id, fxo_box_id) DO UPDATE SET
			can_dial = excluded.can_dial,
			can_receive = excluded.can_receive,
			updated_at = excluded.updated_at
	`, userID, boxID, boolToInt(canDial), boolToInt(canReceive), now)
	if err != nil {
		return fmt.Errorf("set user fxo permission: %w", err)
	}
	return nil
}

func (s *Store) SetUserNotifyPreference(ctx context.Context, userID, boxID int64, notify bool) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_fxo_permissions (user_id, fxo_box_id, can_dial, can_receive, notify_incoming, updated_at)
		VALUES (?, ?, 0, 0, ?, ?)
		ON CONFLICT(user_id, fxo_box_id) DO UPDATE SET
			notify_incoming = excluded.notify_incoming,
			updated_at = excluded.updated_at
	`, userID, boxID, boolToInt(notify), now)
	if err != nil {
		return fmt.Errorf("set notify preference: %w", err)
	}
	return nil
}

func (s *Store) ListUserFXOPermissions(ctx context.Context) ([]UserFXOPermission, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, fxo_box_id, can_dial, can_receive, notify_incoming, updated_at
		FROM user_fxo_permissions
		ORDER BY user_id ASC, fxo_box_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list user fxo permissions: %w", err)
	}
	defer rows.Close()

	out := make([]UserFXOPermission, 0)
	for rows.Next() {
		var item UserFXOPermission
		var canDial, canReceive, notifyIncoming int
		var updatedAt int64
		if err := rows.Scan(&item.UserID, &item.FXOBoxID, &canDial, &canReceive, &notifyIncoming, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan user fxo permission: %w", err)
		}
		item.CanDial = canDial != 0
		item.CanReceive = canReceive != 0
		item.NotifyIncoming = notifyIncoming != 0
		item.UpdatedAt = time.Unix(updatedAt, 0)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user fxo permissions: %w", err)
	}
	return out, nil
}

func (s *Store) UserCanDialFXO(ctx context.Context, userID, boxID int64, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	return s.userHasFXOPermission(ctx, userID, boxID, "can_dial")
}

func (s *Store) UserCanReceiveFXO(ctx context.Context, userID, boxID int64, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	return s.userHasFXOPermission(ctx, userID, boxID, "can_receive")
}

func (s *Store) ListUsersForIncomingFXO(ctx context.Context, boxID int64) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.role, u.created_at
		FROM users u
		LEFT JOIN user_fxo_permissions p ON p.user_id = u.id AND p.fxo_box_id = ?
		WHERE (u.role = 'admin' OR p.can_receive = 1)
		  AND COALESCE(p.notify_incoming, 1) = 1
		ORDER BY u.id ASC`, boxID)
	if err != nil {
		return nil, fmt.Errorf("list users for incoming fxo: %w", err)
	}
	defer rows.Close()

	out := make([]User, 0)
	for rows.Next() {
		var item User
		var createdAt int64
		if err := rows.Scan(&item.ID, &item.Username, &item.Role, &createdAt); err != nil {
			return nil, fmt.Errorf("scan incoming user: %w", err)
		}
		item.Role = normalizeRole(item.Role)
		item.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incoming users: %w", err)
	}
	return out, nil
}

func (s *Store) userHasFXOPermission(ctx context.Context, userID, boxID int64, field string) (bool, error) {
	if field != "can_dial" && field != "can_receive" {
		return false, fmt.Errorf("unsupported permission field")
	}
	var allowed int
	err := s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT %s FROM user_fxo_permissions WHERE user_id = ? AND fxo_box_id = ?`, field),
		userID, boxID,
	).Scan(&allowed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("get user fxo permission: %w", err)
	}
	return allowed != 0, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case RoleAdmin:
		return RoleAdmin
	default:
		return RoleUser
	}
}
