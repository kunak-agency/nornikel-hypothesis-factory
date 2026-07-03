// Package casefacts деterministically парсит профиль хвостов (Хвосты *.xlsx —
// формат, который команда получила от организаторов хакатона) в
// domain.CaseFacts, минуя LLM. Кейс даёт эти цифры уже посчитанными
// (масса/содержание Ni/Cu по классам крупности и минеральным формам) — заново
// извлекать их пересказом через LLM значит рисковать транскрипционной
// галлюцинацией там, где у нас и так есть точный источник.
//
// Формат не документирован официально и слегка "плавает" между 4 примерами
// кейса (смещение строк, редкие битые формулы "#REF!"), поэтому парсер ищет
// таблицы по заголовкам-меткам ("Класс крупности, мкм", "Элемент N, %/т"), а
// не по фиксированным номерам строк/столбцов.
package casefacts

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"hypothesis-factory/domain"

	"github.com/xuri/excelize/v2"
)

// elementSymbols — организаторы кодируют металлы атомным номером в шапке
// таблицы ("Элемент 28, %" = Ni, "Элемент 29, %" = Cu) вместо явного имени;
// 28/29 — единственные, что реально встречаются в кейсе (Ni, Cu), но парсер
// не падает на незнакомом номере — просто использует "El<N>" как символ.
var elementSymbols = map[int]string{28: "Ni", 29: "Cu"}

var elementHeaderRe = regexp.MustCompile(`Элемент\s*(\d+)`)

func elementSymbol(n int) string {
	if s, ok := elementSymbols[n]; ok {
		return s
	}
	return fmt.Sprintf("El%d", n)
}

type metalCol struct {
	col       int
	symbol    string
	isPercent bool
}

// ParseTailingsExcel читает первый лист .xlsx-файла профиля хвостов и
// возвращает CaseFacts. Возвращает ошибку только если в файле вообще не
// нашлось ни одной таблицы с распознаваемым заголовком ("Элемент N, %/т") —
// частично повреждённые/неполные таблицы парсятся насколько возможно,
// пропущенные ячейки уходят в Warnings, а не молча становятся нулями.
func ParseTailingsExcel(data []byte) (domain.CaseFacts, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return domain.CaseFacts{}, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return domain.CaseFacts{}, fmt.Errorf("xlsx has no sheets")
	}
	// RawCellValue: unformatted numbers ("191330.9"), not the display string
	// with the cell's numFmt applied ("191,330.90") — the thousands-comma
	// display form would otherwise collide with decimal-comma parsing.
	rows, err := f.GetRows(sheets[0], excelize.Options{RawCellValue: true})
	if err != nil {
		return domain.CaseFacts{}, fmt.Errorf("read sheet %q: %w", sheets[0], err)
	}

	facts := domain.CaseFacts{}
	var currentCols []metalCol
	var labelCol int
	tableKind := "" // "" | "totals" | "sizeclass" | "mineralform"
	var mineralFormSizeClass string

	// excludedMineralFormLabels — "Извлекаемый металл"/"Не извлекаемый
	// металл" — это отдельная сводка (сколько потерянного металла в принципе
	// извлекаемо), не ещё одна минеральная форма; в этом же блоке таблицы,
	// без отдельного заголовка, поэтому исключаем по метке, а не по позиции.
	excludedMineralFormLabels := map[string]bool{
		"извлекаемый металл":    true,
		"не извлекаемый металл": true,
	}

	for _, row := range rows {
		if isBlankRow(row) {
			continue
		}
		if cols, lCol, headerLabel, isHeader := detectHeaderRow(row); isHeader {
			currentCols = cols
			labelCol = lCol
			switch {
			case strings.Contains(strings.ToLower(headerLabel), "материал"):
				tableKind = "totals"
			case strings.Contains(strings.ToLower(rowText(row)), "класс крупности"):
				tableKind = "sizeclass"
			default:
				// Заголовок минералогической подтаблицы для одного класса
				// крупности несёт в метке этот же класс (напр. "+125 мкм",
				// "-71 + 45 мкм") вместо содержательного названия таблицы —
				// используем его как тег, не как имя строки.
				tableKind = "mineralform"
				mineralFormSizeClass = headerLabel
			}
			continue
		}
		if currentCols == nil {
			continue
		}

		label := strings.TrimSpace(cellAt(row, labelCol))
		if label == "" {
			continue
		}
		isTotal := strings.HasPrefix(strings.ToLower(label), "итого")

		metalLossPct := map[string]float64{}
		metalTons := map[string]float64{}
		for _, mc := range currentCols {
			raw := cellAt(row, mc.col)
			v, ok := parseFloatLoose(raw)
			if !ok {
				if raw != "" {
					facts.Warnings = append(facts.Warnings,
						fmt.Sprintf("%q: не удалось разобрать значение %q (%s)", label, raw, mc.symbol))
				}
				continue
			}
			if mc.isPercent {
				metalLossPct[mc.symbol] = v
			} else {
				metalTons[mc.symbol] = v
			}
		}

		switch {
		case strings.Contains(strings.ToLower(label), "отвальные хвосты") && !isTotal:
			for sym, t := range metalTons {
				facts.Metals = append(facts.Metals, domain.MetalTotal{Symbol: sym, Tons: t, GradePct: metalLossPct[sym]})
			}
			if v, ok := parseFloatLoose(cellAt(row, labelCol+1)); ok {
				facts.TotalTailingsTons = v
			}
		case tableKind == "sizeclass" && !isTotal:
			sc := domain.SizeClassFact{Label: label, MetalLossPct: metalLossPct, MetalTons: metalTons}
			// "Доля класса, %" — не привязанная к металлу колонка, сразу
			// после labelCol в этой таблице.
			if v, ok := parseFloatLoose(cellAt(row, labelCol+1)); ok {
				sc.MassSharePct = v
			}
			facts.SizeClasses = append(facts.SizeClasses, sc)
		case tableKind == "mineralform" && !isTotal && !excludedMineralFormLabels[strings.ToLower(label)]:
			facts.MineralForms = append(facts.MineralForms, domain.MineralFormFact{
				Label:        label,
				SizeClass:    mineralFormSizeClass,
				MetalLossPct: metalLossPct,
				MetalTons:    metalTons,
			})
		}
	}

	if len(facts.SizeClasses) == 0 && len(facts.MineralForms) == 0 && len(facts.Metals) == 0 {
		return facts, fmt.Errorf("no recognizable case-facts tables found (expected headers like %q)", "Элемент 28, %")
	}
	return facts, nil
}

// detectHeaderRow ищет в строке ячейки вида "Элемент N, %" / "Элемент N, т"
// и возвращает их позиции + символ металла, плюс саму метку-заголовок
// (labelCol — первая непустая ячейка строки; в этом формате колонка A всегда
// пустая разметочная колонка, метка начинается с колонки B).
func detectHeaderRow(row []string) (cols []metalCol, labelCol int, headerLabel string, isHeader bool) {
	labelCol = -1
	for i, cell := range row {
		if labelCol == -1 && strings.TrimSpace(cell) != "" {
			labelCol = i
		}
		m := elementHeaderRe.FindStringSubmatch(cell)
		if m == nil {
			continue
		}
		trimmed := strings.TrimSpace(cell)
		isPercent := strings.HasSuffix(trimmed, "%")
		isTons := strings.HasSuffix(trimmed, ", т")
		if !isPercent && !isTons {
			// "Элемент N" also occurs inside mineral-form row labels that
			// aren't headers at all (e.g. "Пирит/Другие Элемент 29
			// сульфиды") — only a cell ending in the exact unit suffix is a
			// real metric column, not just any cell mentioning the element.
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		cols = append(cols, metalCol{
			col:       i,
			symbol:    elementSymbol(n),
			isPercent: isPercent,
		})
	}
	if len(cols) == 0 {
		return nil, 0, "", false
	}
	if labelCol == -1 {
		labelCol = 1
	}
	return cols, labelCol, strings.TrimSpace(cellAt(row, labelCol)), true
}

func rowText(row []string) string {
	return strings.Join(row, " ")
}

func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// parseFloatLoose разбирает число в русской exсel-выгрузке: обычная
// desятичная запись и научная нотация ("7.26E-2") проходят как есть;
// "#REF!"/пустые/текстовые ячейки — распознанная неудача (ok=false), не
// молчаливый 0.
func parseFloatLoose(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "#") {
		return 0, false
	}
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// BuildLossHotspots ранжирует классы крупности по вкладу в потери металла и
// для каждого из top-N подбирает доминирующую минеральную форму потерь в
// этом классе — тот же формат, что и пример в systemPrompt ProblemSpec-парсера
// ("-71+45 мкм: закрытый Pnt/Cp, ~78% потерь Ni в этом классе"), но посчитан
// кодом по факту из файла, а не придуман LLM по пересказу.
func BuildLossHotspots(facts domain.CaseFacts, topN int) []string {
	type ranked struct {
		sc    domain.SizeClassFact
		metal string
		tons  float64
	}
	var candidates []ranked
	for _, sc := range facts.SizeClasses {
		metal, tons := dominantMetal(sc.MetalTons)
		if metal == "" {
			continue
		}
		candidates = append(candidates, ranked{sc: sc, metal: metal, tons: tons})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].tons > candidates[j].tons })
	if len(candidates) > topN {
		candidates = candidates[:topN]
	}

	formsBySizeClass := map[string][]domain.MineralFormFact{}
	for _, mf := range facts.MineralForms {
		key := normalizeSizeClassLabel(mf.SizeClass)
		formsBySizeClass[key] = append(formsBySizeClass[key], mf)
	}

	var hotspots []string
	for _, c := range candidates {
		forms := formsBySizeClass[normalizeSizeClassLabel(c.sc.Label)]
		var dominantForm string
		var dominantPct float64
		var bestTons float64
		for _, f := range forms {
			if t, ok := f.MetalTons[c.metal]; ok && t > bestTons {
				bestTons = t
				dominantForm = f.Label
				dominantPct = f.MetalLossPct[c.metal]
			}
		}
		if dominantForm != "" {
			hotspots = append(hotspots, fmt.Sprintf("%s мкм: %s, ~%.0f%% потерь %s в этом классе",
				c.sc.Label, dominantForm, dominantPct, c.metal))
		} else {
			hotspots = append(hotspots, fmt.Sprintf("%s мкм: ~%.0f%% потерь %s (%.0f т) сосредоточено в этом классе",
				c.sc.Label, c.sc.MetalLossPct[c.metal], c.metal, c.tons))
		}
	}
	return hotspots
}

func dominantMetal(tons map[string]float64) (string, float64) {
	var bestSym string
	var bestVal float64
	for sym, v := range tons {
		if v > bestVal {
			bestVal = v
			bestSym = sym
		}
	}
	return bestSym, bestVal
}

// normalizeSizeClassLabel сближает варианты написания одного и того же
// класса крупности между таблицей классов ("+125", "-71 + 45") и заголовками
// минералогических подтаблиц ("+125 мкм", " -125 +71 мкм", "-71 + 45 мкм") —
// убирает пробелы и суффикс "мкм", регистронезависимо.
func normalizeSizeClassLabel(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "мкм", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.TrimSpace(s)
}
