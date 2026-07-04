package handlers

import (
	"hypothesis-factory/domain"
	"hypothesis-factory/in"
	"hypothesis-factory/out"
	"hypothesis-factory/pkg/errs"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ListPlants возвращает известные фабрики (по структурированному
// оборудованию) — источник для селектора "выбор фабрики" в UI.
// @Summary      Список известных фабрик
// @Tags         plants
// @Produce      json
// @Success      200  {object}  out.PlantsResponse
// @Failure      500  {object}  errs.Error
// @Router       /plants [get]
func (h *Handler) ListPlants(c *fiber.Ctx) error {
	items, err := h.services.Pipeline.Plants(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(out.PlantsResponse{Items: items})
}

// ListPlantEquipment возвращает оборудование фабрик.
// @Summary      Оборудование фабрик
// @Tags         plants
// @Produce      json
// @Param        plant  query  string  false  "Фильтр по имени фабрики"
// @Success      200  {object}  out.PlantEquipmentListResponse
// @Failure      500  {object}  errs.Error
// @Router       /plant-equipment [get]
func (h *Handler) ListPlantEquipment(c *fiber.Ctx) error {
	items, err := h.services.Pipeline.ListPlantEquipment(c.UserContext(), c.Query("plant"))
	if err != nil {
		return err
	}
	resp := out.PlantEquipmentListResponse{Items: make([]out.PlantEquipmentResponse, 0, len(items)), Total: len(items)}
	for i := range items {
		resp.Items = append(resp.Items, out.PlantEquipmentFromDomain(&items[i]))
	}
	return c.JSON(resp)
}

// CreatePlantEquipment добавляет запись об оборудовании фабрики.
// @Summary      Добавление оборудования фабрики
// @Tags         plants
// @Accept       json
// @Produce      json
// @Param        body  body      in.PlantEquipmentRequest  true  "Запись оборудования"
// @Success      201   {object}  out.PlantEquipmentResponse
// @Failure      422   {object}  errs.Error
// @Failure      500   {object}  errs.Error
// @Router       /plant-equipment [post]
func (h *Handler) CreatePlantEquipment(c *fiber.Ctx) error {
	var body in.PlantEquipmentRequest
	if err := c.BodyParser(&body); err != nil {
		return errs.NewBadRequestError("invalid json")
	}
	if err := h.validate.Struct(&body); err != nil {
		return err
	}
	e := &domain.PlantEquipment{
		PlantName:       body.PlantName,
		PlantAliases:    body.PlantAliases,
		EquipmentType:   body.EquipmentType,
		Model:           body.Model,
		Parameters:      body.Parameters,
		CircuitPosition: body.CircuitPosition,
	}
	if err := h.services.Pipeline.CreatePlantEquipment(c.UserContext(), e); err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(out.PlantEquipmentFromDomain(e))
}

// UpdatePlantEquipment обновляет запись об оборудовании.
// @Summary      Обновление записи оборудования
// @Tags         plants
// @Accept       json
// @Produce      json
// @Param        equipmentId  path      string                    true  "UUID записи"
// @Param        body         body      in.PlantEquipmentRequest  true  "Новые значения"
// @Success      200  {object}  out.PlantEquipmentResponse
// @Failure      404  {object}  errs.Error
// @Failure      422  {object}  errs.Error
// @Router       /plant-equipment/{equipmentId} [put]
func (h *Handler) UpdatePlantEquipment(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("equipmentId"))
	if err != nil {
		return errs.NewValidationError("invalid equipmentId")
	}
	var body in.PlantEquipmentRequest
	if err := c.BodyParser(&body); err != nil {
		return errs.NewBadRequestError("invalid json")
	}
	if err := h.validate.Struct(&body); err != nil {
		return err
	}
	e := &domain.PlantEquipment{
		ID:              id,
		PlantName:       body.PlantName,
		PlantAliases:    body.PlantAliases,
		EquipmentType:   body.EquipmentType,
		Model:           body.Model,
		Parameters:      body.Parameters,
		CircuitPosition: body.CircuitPosition,
	}
	if err := h.services.Pipeline.UpdatePlantEquipment(c.UserContext(), e); err != nil {
		return err
	}
	return c.JSON(out.PlantEquipmentFromDomain(e))
}

// DeletePlantEquipment удаляет запись об оборудовании.
// @Summary      Удаление записи оборудования
// @Tags         plants
// @Param        equipmentId  path  string  true  "UUID записи"
// @Success      204  "Удалено"
// @Failure      404  {object}  errs.Error
// @Router       /plant-equipment/{equipmentId} [delete]
func (h *Handler) DeletePlantEquipment(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("equipmentId"))
	if err != nil {
		return errs.NewValidationError("invalid equipmentId")
	}
	if err := h.services.Pipeline.DeletePlantEquipment(c.UserContext(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
