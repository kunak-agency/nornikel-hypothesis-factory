package out

import (
	"time"

	"hypothesis-factory/domain"
	"hypothesis-factory/repositories"

	"github.com/google/uuid"
)

type PlantEquipmentResponse struct {
	ID              uuid.UUID      `json:"id"`
	PlantName       string         `json:"plantName"`
	PlantAliases    []string       `json:"plantAliases"`
	EquipmentType   string         `json:"equipmentType"`
	Model           string         `json:"model"`
	Parameters      map[string]any `json:"parameters"`
	CircuitPosition string         `json:"circuitPosition"`
	CreatedAt       time.Time      `json:"createdAt"`
}

func PlantEquipmentFromDomain(e *domain.PlantEquipment) PlantEquipmentResponse {
	return PlantEquipmentResponse{
		ID:              e.ID,
		PlantName:       e.PlantName,
		PlantAliases:    e.PlantAliases,
		EquipmentType:   e.EquipmentType,
		Model:           e.Model,
		Parameters:      e.Parameters,
		CircuitPosition: e.CircuitPosition,
		CreatedAt:       e.CreatedAt,
	}
}

type PlantEquipmentListResponse struct {
	Items []PlantEquipmentResponse `json:"items"`
	Total int                      `json:"total"`
}

type PlantsResponse struct {
	Items []repositories.PlantSummary `json:"items"`
}
