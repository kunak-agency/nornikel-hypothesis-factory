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
			Statement:         "Повысить расход собирателя в основной флотации с 40 до 60 г/т",
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

func sampleSources(hyps []domain.Hypothesis) map[uuid.UUID][]string {
	out := map[uuid.UUID][]string{}
	for _, h := range hyps {
		out[h.ID] = []string{"Абрамов А.А. — Флотационные методы обогащения, 4-е изд., 2016"}
	}
	return out
}

func TestToCSV(t *testing.T) {
	hyps := sampleHypotheses()
	out, err := ToCSV(hyps, sampleSources(hyps))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty csv")
	}
}

func sampleGaps() []string {
	return []string{"По металлу \"Cu\" в извлечённых claims нет явного покрытия"}
}

func TestToPDF(t *testing.T) {
	hyps := sampleHypotheses()
	out, err := ToPDF(sampleSpec(), hyps, sampleSources(hyps), sampleGaps())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 100 || string(out[:4]) != "%PDF" {
		t.Fatalf("output doesn't look like a PDF, len=%d", len(out))
	}
}

func TestToDOCX(t *testing.T) {
	hyps := sampleHypotheses()
	out, err := ToDOCX(sampleSpec(), hyps, sampleSources(hyps), sampleGaps())
	if err != nil {
		t.Fatal(err)
	}
	// .docx is a zip archive — "PK" magic bytes.
	if len(out) < 100 || string(out[:2]) != "PK" {
		t.Fatalf("output doesn't look like a docx/zip, len=%d", len(out))
	}
}
