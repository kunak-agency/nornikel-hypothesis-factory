package in

type CreateRunRequest struct {
	RawText  string         `json:"rawText"  validate:"required" example:"Фабрика: КГМК. Породные хвосты 5824591 СМТ..."`
	RawInput map[string]any `json:"rawInput"`
}
