// Разовый сидер таблицы plant_equipment — структурированное оборудование
// конкретных фабрик, извлечённое вручную из регламентов/схем кейса
// (tools/ingestion/vision_extract/*.md). НЕ автоматический re-run-safe
// импорт: чистит и заново заливает записи с source_document_id из тех же
// документов при каждом запуске (идемпотентно по паре plant_name+model).
//
// Запуск: DATABASE_URL=... go run ./cmd/seed-plant-equipment
package main

import (
	"context"
	"log"

	"hypothesis-factory/domain"
	"hypothesis-factory/repositories"
)

func main() {
	repos, err := repositories.InitRepos()
	if err != nil {
		log.Fatalf("init repos: %v", err)
	}
	if err := repos.MigrateDB(); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ctx := context.Background()

	tofAliases := []string{"ТОФ", "Талнахская обогатительная фабрика", "Талнахская ОФ", "Талнах"}

	// Источник: tools/ingestion/vision_extract/equipment_list.md +
	// scheme_full_grinding_flotation_chain.md +
	// scheme_grinding_classification_layout.md — все три явно говорят про
	// "дробление талнахских руд" / измельчительно-флотационный цех той же
	// схемы, что прямо соответствует ТОФ (Талнахская обогатительная
	// фабрика) — Пример 4 кейса.
	tofEquipment := []domain.PlantEquipment{
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "crusher", Model: "КРД 700/75",
			Parameters: map[string]any{"count": 1}, CircuitPosition: "Дробильный цех, корпус дробления талнахских руд, поз. №4"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "crusher", Model: "КМД-2200Т",
			Parameters: map[string]any{"count": 2}, CircuitPosition: "Дробильный цех, поз. №1, №2"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "screen", Model: "Sibra 2DR 25×60",
			Parameters: map[string]any{"count": 2}, CircuitPosition: "Дробильный цех, поз. №1, №2"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "mill", Model: "МШРГУ 4,5×6,0",
			CircuitPosition: "Измельчительно-флотационный цех, линии 4-3, 5-1, 5-2, 5-4"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "mill", Model: "МШР 3,2×3,8",
			Parameters: map[string]any{"count": 2}, CircuitPosition: "Измельчительно-флотационный цех, линии 4-5, 4-11"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "mill", Model: "МШЦ 4,5×6,0",
			CircuitPosition: "Измельчительно-флотационный цех, линии 4-2, 5-3, 5-5, работает в замкнутом цикле с гидроциклоном ГЦ-660"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "classifier", Model: "Спиральный классификатор 1КСП24М",
			CircuitPosition: "Линия 4-3, поз. 4-3-CF, замкнутый цикл с мельницей МШРГУ 4,5×6,0"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "classifier", Model: "Спиральный классификатор 2КСН24",
			CircuitPosition: "Линии 4-5/4-11 (поз. 4-5.11-CF) и 5-1/5-2 (поз. 5-1.2-CF)"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "hydrocyclone", Model: "ГЦ-660",
			Parameters:      map[string]any{"diameter_mm": 660, "batteries": 3, "configuration": "1 рабочий + 1 резервный на батарею"},
			CircuitPosition: "Линии 4-2, 5-3, 5-5 (поз. 4-2-ГЦ-660, 5-3-ГЦ-660, 5-5-ГЦ-660), замкнутый цикл измельчения"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "thickener", Model: "Сгуститель радиальный П-30",
			Parameters: map[string]any{"count": 6}, CircuitPosition: "поз. 4-СТ-01...06"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "thickener", Model: "Сгуститель радиальный П-50",
			Parameters: map[string]any{"count": 1}, CircuitPosition: "поз. 4-СТ-07"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "flotation_cell", Model: "ФПМ-16-4К",
			Parameters: map[string]any{"count": 29}, CircuitPosition: "поз. 2-1...2-16, 2-19, 2-20, 4-1...4-20 (частично)"},
		{PlantName: "ТОФ", PlantAliases: tofAliases, EquipmentType: "flotation_cell", Model: "ФМ-16УМ-4К",
			Parameters: map[string]any{"count": 4}, CircuitPosition: "поз. 4-4, 4-6, 4-17, 4-18"},
	}

	// Источник: scheme_malomyr_gold_plant_equipment_spec.md — реальный
	// чертёж МР-37/10-2-1235-ТХ, Маломырский рудник (золото, не Ni/Cu —
	// демонстрирует, что механизм не завязан жёстко на один домен).
	malomyrAliases := []string{"Маломырский рудник", "Malomyr", "Маломыр", "Маломырское месторождение"}
	malomyrEquipment := []domain.PlantEquipment{
		{PlantName: "Маломырский рудник", PlantAliases: malomyrAliases, EquipmentType: "flotation_cell", Model: "ТС-70",
			Parameters: map[string]any{"volume_m3": 70, "count": 4}, CircuitPosition: "1-я контрольная флотация (угольная)"},
		{PlantName: "Маломырский рудник", PlantAliases: malomyrAliases, EquipmentType: "flotation_cell", Model: "ТС-160",
			Parameters: map[string]any{"volume_m3": 160, "count": 6}, CircuitPosition: "Основная сульфидная флотация"},
		{PlantName: "Маломырский рудник", PlantAliases: malomyrAliases, EquipmentType: "flotation_cell", Model: "ТС-20",
			Parameters: map[string]any{"volume_m3": 20, "count": 8}, CircuitPosition: "I перечистная флотация"},
		{PlantName: "Маломырский рудник", PlantAliases: malomyrAliases, EquipmentType: "flotation_cell", Model: "ТС-10",
			Parameters: map[string]any{"volume_m3": 10, "count": 6}, CircuitPosition: "II перечистная флотация"},
		{PlantName: "Маломырский рудник", PlantAliases: malomyrAliases, EquipmentType: "thickener", Model: "250-ТН-101",
			Parameters: map[string]any{"diameter_m": 10}, CircuitPosition: "Сгущение сульфидного концентрата"},
		{PlantName: "Маломырский рудник", PlantAliases: malomyrAliases, EquipmentType: "thickener", Model: "250-ТН-102",
			Parameters: map[string]any{"diameter_m": 24}, CircuitPosition: "Сгущение хвостов флотации"},
	}

	all := append(tofEquipment, malomyrEquipment...)
	inserted := 0
	for i := range all {
		if err := repos.PlantEquipment.Create(ctx, &all[i]); err != nil {
			log.Fatalf("insert %s/%s: %v", all[i].PlantName, all[i].Model, err)
		}
		inserted++
	}
	log.Printf("seeded %d plant_equipment rows", inserted)
}
