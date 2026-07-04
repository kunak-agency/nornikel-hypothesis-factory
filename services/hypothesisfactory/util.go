package hypothesisfactory

import "github.com/google/uuid"

// uniqueUUIDs дедуплицирует и разворачивает любое число UUID-срезов — общая
// логика "собрать ID в набор, затем в срез для GetByIDs" (loadEntityReputations,
// GetClaimSources, BuildRunGraph).
func uniqueUUIDs(idLists ...[]uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	var out []uuid.UUID
	for _, ids := range idLists {
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
