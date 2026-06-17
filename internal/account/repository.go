// Package account реализует регистрацию, вход и сессии пользователей.
package account

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrEmailTaken — e-mail уже зарегистрирован.
	ErrEmailTaken = errors.New("account: email already registered")
	// ErrNotFound — пользователь не найден.
	ErrNotFound = errors.New("account: user not found")
)

// User — доменная модель пользователя (без пароля).
type User struct {
	ID        string
	Email     string
	Name      string
	Phone     string
	Role      string
	CreatedAt time.Time
}

// Repository — доступ к пользователям в PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository создаёт репозиторий пользователей.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Create заводит пользователя. Возвращает ErrEmailTaken при дубле e-mail.
func (r *Repository) Create(ctx context.Context, email, passwordHash, name, phone string, consent bool) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var consentAt *time.Time
	if consent {
		now := time.Now().UTC()
		consentAt = &now
	}

	const q = `
		INSERT INTO users (email, password_hash, name, phone, consent_personal_data, consent_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, email, name, phone, role, created_at`
	var u User
	err := r.pool.QueryRow(ctx, q, email, passwordHash, name, phone, consent, consentAt).
		Scan(&u.ID, &u.Email, &u.Name, &u.Phone, &u.Role, &u.CreatedAt)
	if isUniqueViolation(err) {
		return nil, ErrEmailTaken
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Credentials возвращает id, хэш пароля и пользователя по e-mail (для входа).
func (r *Repository) Credentials(ctx context.Context, email string) (passwordHash string, u *User, err error) {
	email = strings.TrimSpace(strings.ToLower(email))
	const q = `
		SELECT id, email, name, phone, role, created_at, password_hash
		FROM users WHERE lower(email) = $1`
	var usr User
	err = r.pool.QueryRow(ctx, q, email).
		Scan(&usr.ID, &usr.Email, &usr.Name, &usr.Phone, &usr.Role, &usr.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	return passwordHash, &usr, nil
}

// GetByID возвращает пользователя по id (для валидации сессии).
func (r *Repository) GetByID(ctx context.Context, id string) (*User, error) {
	const q = `SELECT id, email, name, phone, role, created_at FROM users WHERE id = $1`
	var u User
	err := r.pool.QueryRow(ctx, q, id).Scan(&u.ID, &u.Email, &u.Name, &u.Phone, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// PromoteAdmins назначает роль 'admin' пользователям с указанными e-mail.
// Вызывается на старте сервиса для бутстрапа админов из ADMIN_EMAILS.
func (r *Repository) PromoteAdmins(ctx context.Context, emails []string) error {
	if len(emails) == 0 {
		return nil
	}
	for i, e := range emails {
		emails[i] = strings.TrimSpace(strings.ToLower(e))
	}
	const q = `UPDATE users SET role = 'admin' WHERE lower(email) = ANY($1)`
	_, err := r.pool.Exec(ctx, q, emails)
	return err
}

// PasswordHash возвращает текущий хэш пароля по id пользователя (для смены пароля).
func (r *Repository) PasswordHash(ctx context.Context, id string) (string, error) {
	const q = `SELECT password_hash FROM users WHERE id = $1`
	var hash string
	err := r.pool.QueryRow(ctx, q, id).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return hash, err
}

// UpdatePassword устанавливает новый хэш пароля по id пользователя.
func (r *Repository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	const q = `UPDATE users SET password_hash = $2 WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// isUniqueViolation распознаёт ошибку нарушения уникальности (код 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key")
}
