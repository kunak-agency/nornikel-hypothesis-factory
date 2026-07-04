package hypothesisfactory

import (
	"fmt"
	"strings"
	"unicode"

	"hypothesis-factory/domain"
)

// metalStems — русские основы названий металлов: учебники пишут "извлечение
// меди", а не "извлечение Cu", и проверка покрытия только по символу давала
// ложные пробелы. Основы подобраны без коллизий с обиходными словами
// ("никел" не встречается вне никеля; "паллад" вне палладия).
var metalStems = map[string][]string{
	"ni": {"никел"},
	"cu": {"медь", "меди", "медн"},
	"co": {"кобальт"},
	"au": {"золот"},
	"ag": {"серебр"},
	"pt": {"платин"},
	"pd": {"паллад"},
}

// gapTopicStopwords — слова точки потерь, не несущие различительной
// информации для проверки покрытия: boilerplate формата ("в этом классе",
// "потерь"), единицы и имена потоков присутствуют в каждой точке потерь и
// раздували знаменатель word-overlap так, что содержательные слова
// (пирротин, примесь, минеральная форма) теряли вес.
var gapTopicStopwords = map[string]bool{
	"в": true, "и": true, "на": true, "этом": true, "классе": true,
	"потерь": true, "потери": true, "мкм": true, "мм": true,
	"хвосты": true, "хвостах": true, "хвостов": true, "отвальные": true,
}

// detectKnowledgeGaps — детерминированная (не LLM) проверка: для каждого
// металла/точки потерь из ProblemSpec есть ли среди извлечённых в ЭТОМ
// прогоне claims хотя бы один, реально её касающийся. Кейс явно требует
// "выявление пробелов в знаниях" — это честный сигнал "тут evidence мало/
// нет", а не LLM-домысел.
func detectKnowledgeGaps(spec domain.ProblemSpec, claims []domain.Claim) []string {
	haystacks := make([]string, len(claims))
	for i, c := range claims {
		haystacks[i] = normalizeForMatch(c.Subject + " " + c.Metric + " " + c.Quote)
	}

	var gaps []string
	for _, metal := range spec.TargetMetals {
		if metal == "" || metalCovered(metal, haystacks) {
			continue
		}
		gaps = append(gaps, fmt.Sprintf(
			"По металлу %q в извлечённом evidence этого прогона нет явного покрытия — гипотезы по нему опираются на более общие факты, стоит расширить корпус или проверить вручную", metal))
	}
	for _, hotspot := range spec.LossHotspots {
		if hotspot == "" || topicCovered(hotspot, haystacks) {
			continue
		}
		gaps = append(gaps, fmt.Sprintf(
			"Точка потерь %q не покрыта извлечённым evidence этого прогона — возможно, требуется отдельное исследование или расширение корпуса", truncate(hotspot, 100)))
	}
	return gaps
}

// metalCovered — металл покрыт, если его символ встречается отдельным словом
// (латиницей, как в формулах/таблицах) либо русская основа названия входит
// подстрокой в текст claim'а.
func metalCovered(symbol string, haystacks []string) bool {
	sym := strings.ToLower(strings.TrimSpace(symbol))
	stems := metalStems[sym]
	for _, h := range haystacks {
		for _, w := range strings.Fields(h) {
			if w == sym {
				return true
			}
		}
		for _, stem := range stems {
			if strings.Contains(h, stem) {
				return true
			}
		}
	}
	return false
}

// topicCovered — точка потерь покрыта, если содержательные слова темы
// (после отбрасывания чисел, единиц и boilerplate формата) пересекаются с
// каким-то одним claim'ом на ≥ трети, либо совпало ≥2 содержательных слова.
func topicCovered(topic string, haystacks []string) bool {
	content := contentWords(topic)
	if len(content) == 0 {
		return true // тема без содержательных слов непроверяема — не пугаем пробелом
	}
	need := 2
	if len(content) < 4 {
		need = 1
	}
	for _, h := range haystacks {
		hayWords := strings.Fields(h)
		hits := 0
		for _, w := range content {
			if matchesAnyWord(w, hayWords) {
				hits++
			}
		}
		if hits >= need || float64(hits)/float64(len(content)) >= 0.34 {
			return true
		}
	}
	return false
}

// matchesAnyWord — совпадение с учётом русской морфологии без стеммера:
// точное равенство либо общий префикс ≥6 букв при том, что одно слово —
// префикс другого ("раскрытый"/"раскрытие", "пирротине"/"пирротин" — одно
// понятие в разных падежах, точное равенство слов их не ловит).
func matchesAnyWord(w string, hay []string) bool {
	for _, hw := range hay {
		if w == hw {
			return true
		}
		if len([]rune(w)) >= 6 && len([]rune(hw)) >= 6 &&
			(strings.HasPrefix(w, hw[:min6(hw)]) || strings.HasPrefix(hw, w[:min6(w)])) {
			return true
		}
	}
	return false
}

// min6 — байтовая длина первых 6 рун слова (для кириллицы руна = 2 байта).
func min6(s string) int {
	n := 0
	for i := range s {
		if n == 6 {
			return i
		}
		n++
	}
	return len(s)
}

// contentWords — нормализованные слова темы без чисел и boilerplate.
func contentWords(topic string) []string {
	var out []string
	for _, w := range strings.Fields(normalizeForMatch(topic)) {
		if gapTopicStopwords[w] || isNumericWord(w) || len([]rune(w)) < 2 {
			continue
		}
		out = append(out, w)
	}
	return out
}

func isNumericWord(w string) bool {
	for _, r := range w {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
