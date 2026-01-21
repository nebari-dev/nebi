package cliclient

import (
	"context"
	"fmt"
)

// ListRegistries returns all registries (tries admin endpoint first, then public).
func (c *Client) ListRegistries(ctx context.Context) ([]Registry, error) {
	var registries []Registry
	_, err := c.Get(ctx, "/admin/registries", &registries)
	if err != nil {
		// Try public endpoint
		_, err = c.Get(ctx, "/registries", &registries)
		if err != nil {
			return nil, err
		}
	}
	return registries, nil
}

// ListRegistriesAdmin returns all registries (admin endpoint only).
func (c *Client) ListRegistriesAdmin(ctx context.Context) ([]Registry, error) {
	var registries []Registry
	_, err := c.Get(ctx, "/admin/registries", &registries)
	if err != nil {
		return nil, err
	}
	return registries, nil
}

// ListRegistriesPublic returns all registries (public endpoint only).
func (c *Client) ListRegistriesPublic(ctx context.Context) ([]Registry, error) {
	var registries []Registry
	_, err := c.Get(ctx, "/registries", &registries)
	if err != nil {
		return nil, err
	}
	return registries, nil
}

// CreateRegistry creates a new registry.
func (c *Client) CreateRegistry(ctx context.Context, req CreateRegistryRequest) (*Registry, error) {
	var registry Registry
	_, err := c.Post(ctx, "/admin/registries", req, &registry)
	if err != nil {
		return nil, err
	}
	return &registry, nil
}

// UpdateRegistry updates a registry.
func (c *Client) UpdateRegistry(ctx context.Context, id string, req UpdateRegistryRequest) (*Registry, error) {
	var registry Registry
	_, err := c.Put(ctx, fmt.Sprintf("/admin/registries/%s", id), req, &registry)
	if err != nil {
		return nil, err
	}
	return &registry, nil
}

// DeleteRegistry deletes a registry by ID.
func (c *Client) DeleteRegistry(ctx context.Context, id string) error {
	_, err := c.Delete(ctx, fmt.Sprintf("/admin/registries/%s", id))
	return err
}
