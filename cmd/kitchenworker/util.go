package kitchenworker

import "strings"

// detectWorkerType returns a canonical, comma+space separated list of supported order types.
func detectWorkerType(orderTypes *string) string {
	allowed := map[string]struct{}{
		"dine_in":  {},
		"takeout":  {},
		"delivery": {},
	}
	order := []string{"dine_in", "takeout", "delivery"}

	// Helper to render canonical list from a presence set.
	render := func(have map[string]bool) string {
		var out []string
		for _, k := range order {
			if have[k] {
				out = append(out, k)
			}
		}
		// Join with comma+space for readability/consistency.
		return strings.Join(out, ", ")
	}

	// Default: all supported.
	if orderTypes == nil || strings.TrimSpace(*orderTypes) == "" {
		return render(map[string]bool{
			"dine_in":  true,
			"takeout":  true,
			"delivery": true,
		})
	}

	parts := strings.Split(*orderTypes, ",")
	have := map[string]bool{}

	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := allowed[p]; ok {
			have[p] = true
		}
	}

	// If user input yields nothing valid, treat as "all supported".
	if len(have) == 0 {
		return render(map[string]bool{
			"dine_in":  true,
			"takeout":  true,
			"delivery": true,
		})
	}

	return render(have)
}
