package main

import (
	"context"
	"fmt"

	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/db"
	"github.com/aktech/darb/internal/models"
	"gorm.io/gorm"
)

// App struct holds application state for the desktop app
type App struct {
	ctx    context.Context
	db     *gorm.DB
	config *config.Config
}

// NewApp creates a new App instance
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("Warning: Could not load config:", err)
		return
	}
	a.config = cfg

	// Connect to database
	database, err := db.New(cfg.Database)
	if err != nil {
		fmt.Println("Warning: Could not connect to database:", err)
		return
	}
	a.db = database
}

// Environment represents a simplified environment for the frontend
type Environment struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	PackageManager string `json:"packageManager"`
	CreatedAt      string `json:"createdAt"`
}

// ListEnvironments returns all environments
func (a *App) ListEnvironments() ([]Environment, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	var envs []models.Environment
	if err := a.db.Order("created_at DESC").Find(&envs).Error; err != nil {
		return nil, err
	}

	result := make([]Environment, len(envs))
	for i, env := range envs {
		result[i] = Environment{
			ID:             env.ID.String(),
			Name:           env.Name,
			Status:         string(env.Status),
			PackageManager: env.PackageManager,
			CreatedAt:      env.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}
	return result, nil
}

// CreateEnvironment creates a new environment
func (a *App) CreateEnvironment(name string, pixiToml string) (*Environment, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	env := models.Environment{
		Name:           name,
		Status:         models.EnvStatusPending,
		PackageManager: "pixi",
	}

	if err := a.db.Create(&env).Error; err != nil {
		return nil, err
	}

	return &Environment{
		ID:             env.ID.String(),
		Name:           env.Name,
		Status:         string(env.Status),
		PackageManager: env.PackageManager,
		CreatedAt:      env.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

// DeleteEnvironment deletes an environment by ID
func (a *App) DeleteEnvironment(id string) error {
	if a.db == nil {
		return fmt.Errorf("database not connected")
	}

	return a.db.Where("id = ?", id).Delete(&models.Environment{}).Error
}

// GetEnvironment gets a single environment by ID
func (a *App) GetEnvironment(id string) (*Environment, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	var env models.Environment
	if err := a.db.Where("id = ?", id).First(&env).Error; err != nil {
		return nil, err
	}

	return &Environment{
		ID:             env.ID.String(),
		Name:           env.Name,
		Status:         string(env.Status),
		PackageManager: env.PackageManager,
		CreatedAt:      env.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}
