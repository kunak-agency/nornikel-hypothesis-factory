package export

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"hypothesis-factory/domain"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

// Шрифты DejaVu Sans встроены в бинарник (go:embed), а не подгружаются с
// диска — иначе PDF-экспорт ломается на любой машине без системных шрифтов
// с кириллицей (в т.ч. в судейском/CI-окружении). DejaVu — public domain
// (Bitstream Vera Fonts License), лицензия не требует атрибуции.
//
//go:embed assets/fonts/DejaVuSans.ttf
var dejaVuRegular []byte

//go:embed assets/fonts/DejaVuSans-Bold.ttf
var dejaVuBold []byte

const (
	pdfFontFamily = "DejaVu"
	pdfMarginMM   = 15.0
)

// ToPDF рендерит тот же отчёт, что и Markdown/DOCX-версии, в PDF —
// карточками гипотез с оценками, а не сплошным текстом. sources (может
// быть nil) — hypothesis.ID -> названия документов-источников evidence, см.
// hypothesisfactory.BuildEvidenceSources. knowledgeGaps — из
// HypothesisRun.KnowledgeGaps, тоже может быть nil.
func ToPDF(spec domain.ProblemSpec, hyps []domain.Hypothesis, sources map[uuid.UUID][]string, knowledgeGaps []string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(pdfFontFamily, "", dejaVuRegular)
	pdf.AddUTF8FontFromBytes(pdfFontFamily, "B", dejaVuBold)
	pdf.SetMargins(pdfMarginMM, pdfMarginMM, pdfMarginMM)
	pdf.SetAutoPageBreak(true, pdfMarginMM)
	pdf.AddPage()

	pageWidth, _ := pdf.GetPageSize()
	contentWidth := pageWidth - 2*pdfMarginMM

	pdf.SetFont(pdfFontFamily, "B", 18)
	pdf.MultiCell(contentWidth, 9, "Фабрика гипотез — отчёт", "", "L", false)
	pdf.Ln(2)

	pdf.SetFont(pdfFontFamily, "B", 11)
	writeLabeledLine(pdf, contentWidth, "Цель: ", spec.TargetKPI)
	if spec.Plant != "" {
		writeLabeledLine(pdf, contentWidth, "Фабрика: ", spec.Plant)
	}
	if len(spec.TargetMetals) > 0 {
		writeLabeledLine(pdf, contentWidth, "Целевые металлы: ", strings.Join(spec.TargetMetals, ", "))
	}
	if len(spec.LossHotspots) > 0 {
		pdf.SetFont(pdfFontFamily, "B", 11)
		pdf.MultiCell(contentWidth, 6, "Точки потерь:", "", "L", false)
		writeBulletList(pdf, contentWidth, spec.LossHotspots)
	}
	if len(spec.Constraints) > 0 {
		pdf.SetFont(pdfFontFamily, "B", 11)
		pdf.MultiCell(contentWidth, 6, "Ограничения:", "", "L", false)
		writeBulletList(pdf, contentWidth, spec.Constraints)
	}
	if len(knowledgeGaps) > 0 {
		pdf.SetFont(pdfFontFamily, "B", 11)
		pdf.MultiCell(contentWidth, 6, "Пробелы в знаниях (слабое покрытие evidence):", "", "L", false)
		writeBulletList(pdf, contentWidth, knowledgeGaps)
	}
	pdf.Ln(4)
	drawSeparator(pdf, contentWidth)

	for _, h := range hyps {
		pdf.SetFont(pdfFontFamily, "B", 13)
		pdf.MultiCell(contentWidth, 7, fmt.Sprintf("%d. %s", h.Rank, h.Statement), "", "L", false)

		writeLabeledParagraph(pdf, contentWidth, "Механизм: ", h.Mechanism)
		writeLabeledParagraph(pdf, contentWidth, "Ожидаемый эффект на KPI: ",
			fmt.Sprintf("%s — %s (%s)", h.ExpectedKPIEffect.Metric, h.ExpectedKPIEffect.Direction, h.ExpectedKPIEffect.Magnitude))
		if h.NoveltyReason != "" {
			writeLabeledParagraph(pdf, contentWidth, "Новизна: ", h.NoveltyReason)
		}
		if len(h.Risks) > 0 {
			pdf.SetFont(pdfFontFamily, "B", 11)
			pdf.MultiCell(contentWidth, 6, "Риски:", "", "L", false)
			writeBulletList(pdf, contentWidth, h.Risks)
		}
		if len(h.VerificationPlan) > 0 {
			pdf.SetFont(pdfFontFamily, "B", 11)
			pdf.MultiCell(contentWidth, 6, "Дорожная карта проверки:", "", "L", false)
			pdf.SetFont(pdfFontFamily, "", 11)
			for i, v := range h.VerificationPlan {
				pdf.MultiCell(contentWidth, 6,
					fmt.Sprintf("%d. %s (ресурсы: %s; критерий успеха: %s)", i+1, v.Step, v.Resource, v.SuccessCrit),
					"", "L", false)
			}
			pdf.Ln(1)
		}

		writeLabeledParagraph(pdf, contentWidth, "Оценки: ", fmt.Sprintf(
			"evidence=%.1f, feasibility=%.1f, impact=%.1f, novelty=%.1f, risk_penalty=%.1f, confidence=%.1f, итого=%.1f",
			h.Scores.EvidenceStrength, h.Scores.Feasibility, h.Scores.Impact, h.Scores.Novelty,
			h.Scores.RiskPenalty, h.Scores.Confidence, h.Scores.Total))
		if h.CriticNotes != "" {
			writeLabeledParagraph(pdf, contentWidth, "Замечание рецензента: ", h.CriticNotes)
		}
		if titles := sources[h.ID]; len(titles) > 0 {
			writeLabeledParagraph(pdf, contentWidth, fmt.Sprintf("Источники (%d ссылок на evidence): ", len(h.EvidenceRefs)), strings.Join(titles, "; "))
		} else {
			writeLabeledParagraph(pdf, contentWidth, "Ссылок на evidence: ", fmt.Sprintf("%d", len(h.EvidenceRefs)))
		}

		pdf.Ln(2)
		drawSeparator(pdf, contentWidth)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return buf.Bytes(), nil
}

func writeLabeledLine(pdf *fpdf.Fpdf, width float64, label, value string) {
	pdf.SetFont(pdfFontFamily, "B", 11)
	pdf.CellFormat(pdf.GetStringWidth(label)+1, 6, label, "", 0, "L", false, 0, "")
	pdf.SetFont(pdfFontFamily, "", 11)
	pdf.MultiCell(width-pdf.GetStringWidth(label)-1, 6, value, "", "L", false)
}

func writeLabeledParagraph(pdf *fpdf.Fpdf, width float64, label, value string) {
	pdf.SetFont(pdfFontFamily, "B", 11)
	pdf.MultiCell(width, 6, label, "", "L", false)
	pdf.SetFont(pdfFontFamily, "", 11)
	pdf.MultiCell(width, 6, value, "", "L", false)
	pdf.Ln(1)
}

func writeBulletList(pdf *fpdf.Fpdf, width float64, items []string) {
	pdf.SetFont(pdfFontFamily, "", 11)
	for _, it := range items {
		pdf.MultiCell(width, 6, "• "+it, "", "L", false)
	}
	pdf.Ln(1)
}

func drawSeparator(pdf *fpdf.Fpdf, width float64) {
	x, y := pdf.GetXY()
	pdf.Line(x, y, x+width, y)
	pdf.Ln(4)
}

