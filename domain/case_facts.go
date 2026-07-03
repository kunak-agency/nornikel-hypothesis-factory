package domain

// CaseFacts — детерминированно распарсенные числа из профиля хвостов
// (Хвосты *.xlsx: масса, содержание Ni/Cu, разбивка по классам крупности и
// минеральным формам). В отличие от ProblemSpec, эти поля никогда не проходят
// через LLM — цифры берутся из файла как есть, чтобы устранить риск
// транскрипционной ошибки/галлюцинации при пересказе таблицы текстом.
type CaseFacts struct {
	TotalTailingsTons float64
	Metals            []MetalTotal
	SizeClasses       []SizeClassFact
	MineralForms      []MineralFormFact
	// Warnings — например "#REF! в 'Раскрытый Pnt/Cp' (Cu)": ячейка не
	// содержала числа, значение пропущено (не подставлен 0 молча), чтобы
	// сломанные формулы источника не притворялись достоверным нулевым fact.
	Warnings []string
}

type MetalTotal struct {
	Symbol   string // "Ni" | "Cu" | "El<N>" для неизвестных номеров элемента
	GradePct float64
	Tons     float64
}

// SizeClassFact — строка таблицы "Класс крупности, мкм" с долей металла,
// потерянного в этом классе.
type SizeClassFact struct {
	Label        string
	MassSharePct float64
	MetalLossPct map[string]float64 // symbol -> доля потерь металла в этом классе, %
	MetalTons    map[string]float64 // symbol -> тонны металла в этом классе
}

// MineralFormFact — строка минералогической разбивки потерь (раскрытый/
// закрытый Pnt/Cp, силикатная форма, миллерит и т.д.) для одного класса
// крупности — источник даёт эту разбивку отдельной подтаблицей на каждый
// класс, а не одной сводной таблицей.
type MineralFormFact struct {
	Label        string
	SizeClass    string
	MetalLossPct map[string]float64
	MetalTons    map[string]float64
}
