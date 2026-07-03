package export

import (
	"testing"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

func sampleSpec() domain.ProblemSpec {
	return domain.ProblemSpec{
		TargetKPI:    "снижение потерь Ni в породных хвостах",
		Plant:        "КГМК",
		TargetMetals: []string{"Ni", "Cu"},
		LossHotspots: []string{"-71+45 мкм: закрытый Pnt/Cp, ~78% потерь Ni в этом классе"},
		Constraints:  []string{"без остановки цепи более 4ч"},
	}
}

func sampleHypotheses() []domain.Hypothesis {
	return []domain.Hypothesis{
		{
			Rank:              1,
			Statement:         "Уменьшить диаметр насадки гидроциклона с 12 до 8 мм",
			Mechanism:         "снижает граничную крупность разделения, доразмалывая закрытые сростки",
			ExpectedKPIEffect: domain.KPIEffect{Metric: "извлечение Ni", Direction: "increase", Magnitude: "+3-5%"},
			Risks:             []string{"рост нагрузки на мельницу"},
			NoveltyReason:     "не применялось на этой фабрике",
			VerificationPlan: []domain.VerificationStep{
				{Step: "пилотный тест на одной нитке", Resource: "1 неделя, лаборатория", SuccessCrit: "снижение потерь Ni >= 2%"},
			},
			Scores:       domain.Scores{EvidenceStrength: 4, Feasibility: 4, Impact: 5, Novelty: 3, RiskPenalty: 1, Confidence: 4, Total: 4.2},
			CriticNotes:  "требует проверки на реальном сырье",
			EvidenceRefs: []uuid.UUID{uuid.New(), uuid.New()},
		},
	}
}

func TestToCSV(t *testing.T) {
	out, err := ToCSV(sampleHypotheses())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty csv")
	}
}

func TestToPDF(t *testing.T) {
	out, err := ToPDF(sampleSpec(), sampleHypotheses())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 100 || string(out[:4]) != "%PDF" {
		t.Fatalf("output doesn't look like a PDF, len=%d", len(out))
	}
}

func TestToDOCX(t *testing.T) {
	out, err := ToDOCX(sampleSpec(), sampleHypotheses())
	if err != nil {
		t.Fatal(err)
	}
	// .docx is a zip archive — "PK" magic bytes.
	if len(out) < 100 || string(out[:2]) != "PK" {
		t.Fatalf("output doesn't look like a docx/zip, len=%d", len(out))
	}
}
