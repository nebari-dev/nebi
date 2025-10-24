package main

import (
	"fmt"
	"log"
	"os"

	"github.com/aktech/darb/internal/auth"
	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/db"
	"github.com/aktech/darb/internal/models"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run scripts/create_user.go <username> <email> <password>")
		os.Exit(1)
	}

	username := os.Args[1]
	email := os.Args[2]
	password := os.Args[3]

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	database, err := db.New(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Hash password
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Create user
	user := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
	}

	result := database.Create(&user)
	if result.Error != nil {
		log.Fatalf("Failed to create user: %v", result.Error)
	}

	fmt.Printf("User created successfully!\n")
	fmt.Printf("ID: %d\n", user.ID)
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Email: %s\n", user.Email)
}
