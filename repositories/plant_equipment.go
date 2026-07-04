package repositories

import (
	"context"
	"strings"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
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

// List возвращает оборудование, опционально фильтруя по фабрике (пустая
// строка = все записи).
func (r *PlantEquipmentRepo) List(ctx context.Context, plantName string) ([]domain.PlantEquipment, error) {
	q := r.db.WithContext(ctx).Order("plant_name, equipment_type, model")
	if plantName != "" {
		q = q.Where("plant_name = ?", plantName)
	}
	var out []domain.PlantEquipment
	err := q.Find(&out).Error
	return out, err
}

// Update перезаписывает изменяемые поля записи. Возвращает число затронутых
// строк (0 = не найдена).
func (r *PlantEquipmentRepo) Update(ctx context.Context, e *domain.PlantEquipment) (int64, error) {
	res := r.db.WithContext(ctx).Model(&domain.PlantEquipment{}).Where("id = ?", e.ID).
		Updates(map[string]any{
			"plant_name":       e.PlantName,
			"plant_aliases":    e.PlantAliases,
			"equipment_type":   e.EquipmentType,
			"model":            e.Model,
			"parameters":       e.Parameters,
			"circuit_position": e.CircuitPosition,
		})
	return res.RowsAffected, res.Error
}

// Delete удаляет запись по id. Возвращает число удалённых строк.
func (r *PlantEquipmentRepo) Delete(ctx context.Context, id uuid.UUID) (int64, error) {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&domain.PlantEquipment{})
	return res.RowsAffected, res.Error
}

// Plants — список известных фабрик (различные plant_name) с числом позиций
// оборудования — то, из чего UI строит селектор "выбор фабрики".
type PlantSummary struct {
	PlantName      string `json:"plantName"`
	EquipmentCount int    `json:"equipmentCount"`
}

func (r *PlantEquipmentRepo) Plants(ctx context.Context) ([]PlantSummary, error) {
	var out []PlantSummary
	err := r.db.WithContext(ctx).Model(&domain.PlantEquipment{}).
		Select("plant_name, count(*) AS equipment_count").
		Group("plant_name").Order("plant_name").Scan(&out).Error
	return out, err
}
