package hypothesisfactory

import "github.com/google/uuid"

// uniqueUUIDs dedupes and flattens any number of UUID slices — the shared
// body of "collect referenced IDs into a set, then a slice for a GetByIDs
// call" that recurs across entity/claim lookups (loadEntityReputations,
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
