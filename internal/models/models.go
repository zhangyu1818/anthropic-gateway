package models

import (
	"time"

	"anthropic-gateway/internal/config"
)

type ListResponse struct {
	Data    []Model `json:"data"`
	FirstID string  `json:"first_id,omitempty"`
	LastID  string  `json:"last_id,omitempty"`
	HasMore bool    `json:"has_more"`
}

type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

func BuildListResponse(cfg *config.Config) ListResponse {
	items := make([]Model, 0, len(cfg.ModelList))
	created := time.Now().UTC().Format(time.RFC3339)
	for _, route := range cfg.ModelList {
		items = append(items, Model{
			ID:          route.ModelName,
			Type:        "model",
			DisplayName: route.ModelName,
			CreatedAt:   created,
		})
	}

	resp := ListResponse{
		Data:    items,
		HasMore: false,
	}
	if len(items) > 0 {
		resp.FirstID = items[0].ID
		resp.LastID = items[len(items)-1].ID
	}
	return resp
}
