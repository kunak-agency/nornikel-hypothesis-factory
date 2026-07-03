package casefacts

import (
	"os"
	"testing"
)

// Смок-тест против реальных файлов кейса — не юнит-тест на синтетике, потому
// что вся сложность парсера как раз в том, что реальный формат "плавает"
// между примерами (смещение строк, битые #REF! формулы в Примере 1).
func TestParseTailingsExcel_RealCaseFiles(t *testing.T) {
	// Точное число строк в источнике "плавает" между примерами (Пример 4
	// профилирует два потока хвостов — породные и пирротиновые — вместо
	// одного), так что проверяем структурную состоятельность, не точные
	// счётчики.
	cases := []struct {
		path          string
		wantMinMetals int
		wantMinClass  int
	}{
		{"/home/god/Документы/nornikel/Задача 1/Пример 1/Хвосты КГМК.xlsx", 2, 6},
		{"/home/god/Документы/nornikel/Задача 1/Пример 2/Хвосты НОФ Вкр.xlsx", 2, 6},
		{"/home/god/Документы/nornikel/Задача 1/Пример 3/Хвосты НОФ мед.xlsx", 2, 6},
		{"/home/god/Документы/nornikel/Задача 1/Пример 4/Хвосты ТОФ_2.xlsx", 2, 6},
	}

	for _, tc := range cases {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Skipf("case file not available: %v", err)
			continue
		}
		facts, err := ParseTailingsExcel(data)
		if err != nil {
			t.Fatalf("%s: ParseTailingsExcel: %v", tc.path, err)
		}
		if len(facts.Metals) < tc.wantMinMetals {
			t.Errorf("%s: got %d metals, want >= %d", tc.path, len(facts.Metals), tc.wantMinMetals)
		}
		if len(facts.SizeClasses) < tc.wantMinClass {
			t.Errorf("%s: got %d size classes, want >= %d", tc.path, len(facts.SizeClasses), tc.wantMinClass)
		}
		if facts.TotalTailingsTons <= 0 {
			t.Errorf("%s: TotalTailingsTons = %v, want > 0", tc.path, facts.TotalTailingsTons)
		}
		for _, sc := range facts.SizeClasses {
			if sc.Label == "" {
				t.Errorf("%s: size class with empty label", tc.path)
			}
		}
		if len(facts.MineralForms) == 0 {
			t.Errorf("%s: no mineral forms parsed", tc.path)
		}
		t.Logf("%s: metals=%+v sizeClasses=%d mineralForms=%d warnings=%d",
			tc.path, facts.Metals, len(facts.SizeClasses), len(facts.MineralForms), len(facts.Warnings))
		for _, w := range facts.Warnings {
			t.Logf("  warning: %s", w)
		}
	}
}

func TestBuildLossHotspots(t *testing.T) {
	data, err := os.ReadFile("/home/god/Документы/nornikel/Задача 1/Пример 3/Хвосты НОФ мед.xlsx")
	if err != nil {
		t.Skipf("case file not available: %v", err)
	}
	facts, err := ParseTailingsExcel(data)
	if err != nil {
		t.Fatalf("ParseTailingsExcel: %v", err)
	}
	hotspots := BuildLossHotspots(facts, 3)
	if len(hotspots) == 0 {
		t.Fatal("expected at least one hotspot")
	}
	for _, h := range hotspots {
		t.Logf("hotspot: %s", h)
	}
}
