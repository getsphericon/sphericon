package bad

var allowed = []string{"contacts:read", "contacts:write", "events:read"}

const maxNameLength = 137

func ok(s string) bool {
	switch s {
	case "contacts:read", "contacts:write", "events:read":
		return true
	}
	return false
}

// contract-copy:
const maxPage = 250
