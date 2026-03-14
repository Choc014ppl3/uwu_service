package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/windfall/uwu_service/internal/infra/client"
)

// UserRepository struct
type userRepository struct {
	db  *client.PostgresClient
	log *slog.Logger
}

// NewUserRepository constructor
func NewUserRepository(db *client.PostgresClient, log *slog.Logger) *userRepository {
	return &userRepository{db: db, log: log}
}

// Create inserts a new user into the database.
func (r *userRepository) CreateUser(ctx context.Context, user *User) error {
	if r.db == nil || r.db.Pool == nil {
		err := fmt.Errorf("database not configured")
		r.log.ErrorContext(ctx, "database connection missing", slog.Any("error", err))
		return err
	}

	query := `
        INSERT INTO users (email, password_hash, display_name, avatar_url, bio, settings)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, created_at, updated_at
    `

	settingsDB := user.Settings
	if len(settingsDB) == 0 {
		settingsDB = []byte("{}")
	}

	err := r.db.Pool.QueryRow(ctx, query,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.AvatarURL,
		user.Bio,
		settingsDB,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		// บันทึก Log ว่า Query พัง พร้อมบอกด้วยว่าพังที่ Email ไหน
		r.log.ErrorContext(ctx, "failed to insert user into database",
			slog.Any("error", err),
			slog.String("email", user.Email),
		)
		return fmt.Errorf("failed to create user: %w", err)
	}

	// (Optional) ถ้าอยากให้เห็นว่า สร้าง User สำเร็จ ก็สามารถใส่ InfoContext ได้
	// แต่ในระดับ Prod มักจะไม่ใส่ Info ใน Repo เพื่อลดความรกของ Log ครับ
	// r.log.DebugContext(ctx, "user created successfully", slog.Int("user_id", user.ID))

	return nil
}

// GetByEmail retrieves a user by email address.
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	if r.db == nil || r.db.Pool == nil {
		err := fmt.Errorf("database not configured")
		r.log.ErrorContext(ctx, "database connection missing", slog.Any("error", err))
		return nil, err
	}

	query := `
        SELECT id, email, password_hash, display_name, avatar_url, bio, settings, created_at, updated_at
        FROM users
        WHERE email = $1
    `

	var user User
	err := r.db.Pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.AvatarURL,
		&user.Bio,
		&user.Settings,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		// ถ้าหาไม่เจอ (ErrNoRows) เป็นเรื่องปกติทาง Business Logic ไม่ถือว่าเป็น System Error จึงไม่ต้อง Log Error
		if err == pgx.ErrNoRows {
			return nil, nil
		}

		// แต่ถ้าเป็น Error อื่นๆ เช่น DB ล่ม, Query ผิด อันนี้ต้อง Log เก็บไว้ดู
		r.log.ErrorContext(ctx, "failed to query user by email",
			slog.Any("error", err),
			slog.String("email", email),
		)
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return &user, nil
}
