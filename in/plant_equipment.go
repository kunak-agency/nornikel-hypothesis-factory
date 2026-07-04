package in

// PlantEquipmentRequest — создание/обновление записи об оборудовании фабрики.
type PlantEquipmentRequest struct {
	PlantName    string   `json:"plantName"    validate:"required"        example:"ТОФ"`
	PlantAliases []string `json:"plantAliases"                            example:"Талнахская обогатительная фабрика,Талнах"`
	// EquipmentType — тип аппарата; известные типы получают собственный
	// retrieval-фасет с рычагами (см. equipmentTypeLevers).
	EquipmentType   string         `json:"equipmentType" validate:"required,oneof=hydrocyclone mill classifier screen flotation_cell thickener crusher pump" example:"hydrocyclone"`
	Model           string         `json:"model"         validate:"required" example:"ГЦ-660"`
	Parameters      map[string]any `json:"parameters"`
	CircuitPosition string         `json:"circuitPosition" example:"Линии 4-2, 5-3, 5-5, замкнутый цикл измельчения"`
}
