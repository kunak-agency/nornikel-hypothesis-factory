package in

type SubmitFeedbackRequest struct {
	Verdict  string `json:"verdict"  validate:"required,oneof=confirmed rejected needs_revision" example:"confirmed"`
	Comment  string `json:"comment"  validate:"omitempty,max=2000"`
	Reviewer string `json:"reviewer" validate:"omitempty,max=200"`
}
