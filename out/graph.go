package out

import "hypothesis-factory/services/hypothesisfactory"

type GraphNodeResponse struct {
	ID    string `json:"id"`
	Type  string `json:"type" example:"entity"` // entity | claim | hypothesis
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
}

type GraphEdgeResponse struct {
	ID    string `json:"id"`
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type" example:"evidence"` // subject | affects | evidence
	Label string `json:"label,omitempty"`
}

type GraphResponse struct {
	Nodes []GraphNodeResponse `json:"nodes"`
	Edges []GraphEdgeResponse `json:"edges"`
}

func GraphFromDomain(g hypothesisfactory.Graph) GraphResponse {
	resp := GraphResponse{
		Nodes: make([]GraphNodeResponse, 0, len(g.Nodes)),
		Edges: make([]GraphEdgeResponse, 0, len(g.Edges)),
	}
	for _, n := range g.Nodes {
		resp.Nodes = append(resp.Nodes, GraphNodeResponse{ID: n.ID, Type: n.Type, Label: n.Label, Kind: n.Kind})
	}
	for _, e := range g.Edges {
		resp.Edges = append(resp.Edges, GraphEdgeResponse{ID: e.ID, From: e.From, To: e.To, Type: e.Type, Label: e.Label})
	}
	return resp
}
