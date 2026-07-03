package export

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"hypothesis-factory/domain"

	docx "github.com/fumiama/go-docx"
	"github.com/google/uuid"
)

// docxTemplate — минимальный пустой .docx (сгенерирован LibreOffice).
// docx.New() из этой библиотеки НЕ кладёт обязательные части OOXML-пакета
// ([Content_Types].xml, _rels/.rels, word/styles.xml и т.д.) — Word/
// LibreOffice отказываются открывать такой файл. docx.Parse() от валидного
// пустого шаблона тянет всю эту обвязку из архива, и дальше можно просто
// дописывать параграфы поверх.
//
//go:embed assets/docx/template.docx
var docxTemplate []byte

// ToDOCX рендерит тот же отчёт, что и Markdown/PDF-версии, в .docx —
// карточками гипотез, чтобы можно было открыть и точечно править/
// комментировать в Word перед сдачей.
func ToDOCX(spec domain.ProblemSpec, hyps []domain.Hypothesis, sources map[uuid.UUID][]string, knowledgeGaps []string) ([]byte, error) {
	doc, err := docx.Parse(bytes.NewReader(docxTemplate), int64(len(docxTemplate)))
	if err != nil {
		return nil, fmt.Errorf("parse docx template: %w", err)
	}

	doc.AddParagraph().AddText("Фабрика гипотез — отчёт").Bold().Size("36")

	p := doc.AddParagraph()
	p.AddText("Цель: ").Bold()
	p.AddText(spec.TargetKPI)

	if spec.Plant != "" {
		p := doc.AddParagraph()
		p.AddText("Фабрика: ").Bold()
		p.AddText(spec.Plant)
	}
	if len(spec.TargetMetals) > 0 {
		p := doc.AddParagraph()
		p.AddText("Целевые металлы: ").Bold()
		p.AddText(joinComma(spec.TargetMetals))
	}
	if len(spec.LossHotspots) > 0 {
		doc.AddParagraph().AddText("Точки потерь:").Bold()
		for _, hspot := range spec.LossHotspots {
			doc.AddParagraph().AddText("• " + hspot)
		}
	}
	if len(spec.Constraints) > 0 {
		doc.AddParagraph().AddText("Ограничения:").Bold()
		for _, c := range spec.Constraints {
			doc.AddParagraph().AddText("• " + c)
		}
	}
	if len(knowledgeGaps) > 0 {
		doc.AddParagraph().AddText("Пробелы в знаниях (слабое покрытие evidence):").Bold()
		for _, g := range knowledgeGaps {
			doc.AddParagraph().AddText("• " + g)
		}
	}
	doc.AddParagraph().AddText(separatorLine)

	for _, h := range hyps {
		doc.AddParagraph().AddText(fmt.Sprintf("%d. %s", h.Rank, h.Statement)).Bold().Size("28")

		addLabeledParagraph(doc, "Механизм: ", h.Mechanism)
		addLabeledParagraph(doc, "Ожидаемый эффект на KPI: ",
			fmt.Sprintf("%s — %s (%s)", h.ExpectedKPIEffect.Metric, h.ExpectedKPIEffect.Direction, h.ExpectedKPIEffect.Magnitude))
		if h.NoveltyReason != "" {
			addLabeledParagraph(doc, "Новизна: ", h.NoveltyReason)
		}
		if len(h.Risks) > 0 {
			doc.AddParagraph().AddText("Риски:").Bold()
			for _, r := range h.Risks {
				doc.AddParagraph().AddText("• " + r)
			}
		}
		if len(h.VerificationPlan) > 0 {
			doc.AddParagraph().AddText("Дорожная карта проверки:").Bold()
			for i, v := range h.VerificationPlan {
				doc.AddParagraph().AddText(fmt.Sprintf("%d. %s (ресурсы: %s; критерий успеха: %s)", i+1, v.Step, v.Resource, v.SuccessCrit))
			}
		}

		addLabeledParagraph(doc, "Оценки: ", fmt.Sprintf(
			"evidence=%.1f, feasibility=%.1f, impact=%.1f, novelty=%.1f, risk_penalty=%.1f, confidence=%.1f, итого=%.1f",
			h.Scores.EvidenceStrength, h.Scores.Feasibility, h.Scores.Impact, h.Scores.Novelty,
			h.Scores.RiskPenalty, h.Scores.Confidence, h.Scores.Total))
		if h.CriticNotes != "" {
			addLabeledParagraph(doc, "Замечание рецензента: ", h.CriticNotes)
		}
		if titles := sources[h.ID]; len(titles) > 0 {
			addLabeledParagraph(doc, fmt.Sprintf("Источники (%d ссылок на evidence): ", len(h.EvidenceRefs)), strings.Join(titles, "; "))
		} else {
			addLabeledParagraph(doc, "Ссылок на evidence: ", fmt.Sprintf("%d", len(h.EvidenceRefs)))
		}

		doc.AddParagraph().AddText(separatorLine)
	}

	var buf bytes.Buffer
	if _, err := doc.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("render docx: %w", err)
	}
	return buf.Bytes(), nil
}

const separatorLine = "────────────────────────────────────"

func addLabeledParagraph(doc *docx.Docx, label, value string) {
	p := doc.AddParagraph()
	p.AddText(label).Bold()
	p.AddText(value)
}
