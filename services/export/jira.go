package export

import (
	"encoding/json"
	"fmt"
	"strings"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

// jiraIssueUpdate/jiraFields — тот же payload-формат, что принимает Jira
// Cloud REST API POST /rest/api/2/issue/bulk (issueUpdates: [{fields: {...}}]).
// Экспортируем как файл (нет живого инстанса/ключей для теста), но формат
// намеренно совместим 1:1 — если ключи появятся, этот же JSON уходит прямым
// POST-запросом без перекладки.
type jiraIssueUpdate struct {
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Project     jiraRef  `json:"project"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	IssueType   jiraRef  `json:"issuetype"`
	Priority    jiraRef  `json:"priority,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

type jiraRef struct {
	Key  string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
}

type jiraBulkPayload struct {
	IssueUpdates []jiraIssueUpdate `json:"issueUpdates"`
}

// jiraPriorityByRank — топовые по ранжированию гипотезы получают более
// высокий приоритет проверки в задачнике: это буквально то, для чего нужно
// ранжирование, если результат уходит в трекер задач.
func jiraPriorityByRank(rank int) string {
	switch {
	case rank <= 2:
		return "High"
	case rank <= 5:
		return "Medium"
	default:
		return "Low"
	}
}

// ToJiraJSON рендерит гипотезы как задачи на верификацию — summary/
// description/verification-план из VerificationPlan становится основным
// содержанием задачи (кейс явно требует "экспорт в задачи (CSV, JSON, API
// для внешних систем)"). projectKey — обязателен (Jira issue не может быть
// создан без project); issueType по умолчанию "Task".
func ToJiraJSON(hyps []domain.Hypothesis, sources map[uuid.UUID][]string, projectKey, issueType string) ([]byte, error) {
	if projectKey == "" {
		return nil, fmt.Errorf("projectKey is required")
	}
	if issueType == "" {
		issueType = "Task"
	}

	payload := jiraBulkPayload{IssueUpdates: make([]jiraIssueUpdate, 0, len(hyps))}
	for _, h := range hyps {
		var desc strings.Builder
		fmt.Fprintf(&desc, "*Механизм:* %s\n\n", h.Mechanism)
		fmt.Fprintf(&desc, "*Ожидаемый эффект на KPI:* %s — %s (%s)\n\n",
			h.ExpectedKPIEffect.Metric, h.ExpectedKPIEffect.Direction, h.ExpectedKPIEffect.Magnitude)
		if h.NoveltyReason != "" {
			fmt.Fprintf(&desc, "*Новизна:* %s\n\n", h.NoveltyReason)
		}
		if len(h.Risks) > 0 {
			fmt.Fprintf(&desc, "*Риски:*\n")
			for _, r := range h.Risks {
				fmt.Fprintf(&desc, "- %s\n", r)
			}
			desc.WriteString("\n")
		}
		if len(h.VerificationPlan) > 0 {
			fmt.Fprintf(&desc, "*Дорожная карта проверки:*\n")
			for i, v := range h.VerificationPlan {
				fmt.Fprintf(&desc, "%d. %s (ресурсы: %s; срок: %s; бюджет: %s; критерий успеха: %s)\n",
					i+1, v.Step, v.Resource, orPlaceholder(v.EstimatedDuration), orPlaceholder(v.EstimatedCost), v.SuccessCrit)
			}
			desc.WriteString("\n")
		}
		if titles := sources[h.ID]; len(titles) > 0 {
			fmt.Fprintf(&desc, "*Источники:* %s\n\n", strings.Join(titles, "; "))
		}
		fmt.Fprintf(&desc, "*Оценки:* evidence=%.1f, feasibility=%.1f, impact=%.1f, novelty=%.1f, risk_penalty=%.1f, итого=%.1f",
			h.Scores.EvidenceStrength, h.Scores.Feasibility, h.Scores.Impact, h.Scores.Novelty, h.Scores.RiskPenalty, h.Scores.Total)

		payload.IssueUpdates = append(payload.IssueUpdates, jiraIssueUpdate{
			Fields: jiraFields{
				Project:     jiraRef{Key: projectKey},
				Summary:     fmt.Sprintf("[Гипотеза #%d] %s", h.Rank, h.Statement),
				Description: desc.String(),
				IssueType:   jiraRef{Name: issueType},
				Priority:    jiraRef{Name: jiraPriorityByRank(h.Rank)},
				Labels:      []string{"hypothesis-factory", fmt.Sprintf("rank-%d", h.Rank)},
			},
		})
	}

	return json.MarshalIndent(payload, "", "  ")
}

func orPlaceholder(s string) string {
	if s == "" {
		return "не оценено"
	}
	return s
}
