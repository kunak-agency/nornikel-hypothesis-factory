package in

// Pagination — каноническая структура пагинации запроса.
type Pagination struct {
	Page    int
	PerPage int
}

func NewPagination(page, perPage int) *Pagination {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return &Pagination{Page: page, PerPage: perPage}
}

func (p *Pagination) Offset() int { return (p.Page - 1) * p.PerPage }
func (p *Pagination) Limit() int  { return p.PerPage }
