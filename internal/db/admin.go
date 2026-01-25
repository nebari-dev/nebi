package db

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// CreateDefaultAdmin creates a default admin user if ADMIN_USERNAME and ADMIN_PASSWORD are set
// and no users exist in the database
func CreateDefaultAdmin(db *gorm.DB) error {
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	email := os.Getenv("ADMIN_EMAIL")

	// If no admin credentials provided, skip
	if username == "" || password == "" {
		slog.Info("No ADMIN_USERNAME or ADMIN_PASSWORD set, skipping default admin creation")
		return nil
	}

	// Set default email if not provided
	if email == "" {
		email = fmt.Sprintf("%s@nebi.local", username)
	}

	// Check if any users exist
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}

	// If users already exist, skip
	if count > 0 {
		slog.Info("Users already exist, skipping default admin creation")
		return nil
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
	}

	if err := db.Create(&user).Error; err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	// Initialize RBAC enforcer if not already done
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		return fmt.Errorf("failed to initialize RBAC: %w", err)
	}

	// Grant admin role in RBAC
	if err := rbac.MakeAdmin(user.ID); err != nil {
		return fmt.Errorf("failed to grant admin role: %w", err)
	}

	slog.Info("Default admin user created", "username", username, "email", email)
	return nil
}
