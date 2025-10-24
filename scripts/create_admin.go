package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/db"
	"github.com/aktech/darb/internal/logger"
	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run scripts/create_admin.go <username> <email> <password>")
		os.Exit(1)
	}

	username := os.Args[1]
	email := os.Args[2]
	password := os.Args[3]

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Initialize logger
	logger.Init(cfg.Log.Format, cfg.Log.Level)

	// Initialize database
	database, err := db.New(cfg.Database)
	if err != nil {
		log.Fatal(err)
	}

	// Run migrations to ensure tables exist
	if err := db.Migrate(database); err != nil {
		log.Fatal(err)
	}

	// Initialize RBAC
	if err := rbac.InitEnforcer(database, slog.Default()); err != nil {
		log.Fatal(err)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	user := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
	}

	if err := database.Create(&user).Error; err != nil {
		log.Fatal(err)
	}

	// Grant admin in RBAC
	if err := rbac.MakeAdmin(user.ID); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✅ Admin user created successfully!\n")
	fmt.Printf("ID: %s\n", user.ID)
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Email: %s\n", user.Email)
	fmt.Printf("\nYou can now login with:\n")
	fmt.Printf("  curl -X POST http://localhost:8080/api/v1/auth/login \\\n")
	fmt.Printf("    -H \"Content-Type: application/json\" \\\n")
	fmt.Printf("    -d '{\"username\": \"%s\", \"password\": \"%s\"}'\n", username, password)
}
