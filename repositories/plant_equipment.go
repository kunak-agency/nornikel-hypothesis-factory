package repositories

import (
	"context"
	"strings"

	"hypothesis-factory/domain"

	"gorm.io/gorm"
)

type PlantEquipmentRepo struct{ db *gorm.DB }

func NewPlantEquipmentRepo(db *gorm.DB) *PlantEquipmentRepo { return &PlantEquipmentRepo{db: db} }

func (r *PlantEquipmentRepo) Create(ctx context.Context, e *domain.PlantEquipment) error {
	return r.db.WithContext(ctx).Create(e).Error
}

// DeleteByPlantName удаляет все строки конкретной фабрики — используется
// сидером (cmd/seed-plant-equipment) для реальной идемпотентности повторного
// запуска (clear-then-reload), а не только по заявлению в его шапке.
func (r *PlantEquipmentRepo) DeleteByPlantName(ctx context.Context, plantName string) (int64, error) {
	res := r.db.WithContext(ctx).Where("plant_name = ?", plantName).Delete(&domain.PlantEquipment{})
	return res.RowsAffected, res.Error
}

// FindByPlantMention ищет оборудование, чьё PlantName или один из Aliases
// упоминается (case-insensitive substring) в свободном тексте (обычно —
// ProblemSpec.Plant). Таблица маленькая (десятки строк на фабрику) — простой
// in-memory скан надёжнее и понятнее, чем городить SQL под substring-поиск
// по элементам jsonb-массива aliases.
func (r *PlantEquipmentRepo) FindByPlantMention(ctx context.Context, mention string) ([]domain.PlantEquipment, error) {
	if strings.TrimSpace(mention) == "" {
		return nil, nil
	}
	var all []domain.PlantEquipment
	if err := r.db.WithContext(ctx).Find(&all).Error; err != nil {
		return nil, err
	}
	mentionLower := strings.ToLower(mention)
	var out []domain.PlantEquipment
	for _, e := range all {
		if strings.Contains(mentionLower, strings.ToLower(e.PlantName)) {
			out = append(out, e)
			continue
		}
		for _, alias := range e.PlantAliases {
			if alias != "" && strings.Contains(mentionLower, strings.ToLower(alias)) {
				out = append(out, e)
				break
			}
		}
	}
	return out, nil
}
