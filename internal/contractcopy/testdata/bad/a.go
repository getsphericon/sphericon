package bad

var allowed = []string{"contacts:read", "contacts:write", "events:read"}

func ok(s string) bool {
	switch s {
	case "contacts:read", "contacts:write", "events:read":
		return true
	}
	return false
}
