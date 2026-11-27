package api

import "net/url"

// sessionListSelfService is true for zoa session list / own-session API reads.
// Access Lambda accepts scope=mine (legacy clients) and mine=true (current CLI).
// Break-glass request lists use mine=true only — not this helper.
func sessionListSelfService(q url.Values) bool {
	return q.Get("mine") == "true" || q.Get("scope") == "mine"
}
