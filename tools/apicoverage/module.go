package main

import "strings"

// preferredModuleOrder is the section order in the markdown report.
var preferredModuleOrder = []string{
	"auth",
	"account",
	"users",
	"contacts",
	"messages",
	"updates",
	"photos",
	"upload",
	"help",
	"channels",
	"stickers",
	"phone/calls",
	"langpack",
	"folders",
	"stats",
	"stories",
	"premium/payments",
	"bots",
	"business",
}

// p1Modules are core chat features. Startup and background calls are P0
// regardless of module. Everything else is P2.
var p1Modules = map[string]bool{
	"auth":     true,
	"account":  true,
	"users":    true,
	"contacts": true,
	"messages": true,
	"updates":  true,
	"photos":   true,
	"upload":   true,
	"channels": true,
}

// moduleOf maps a TL method onto a coverage section.
func moduleOf(method string) string {
	ns, name, _ := strings.Cut(method, ".")
	n := strings.ToLower(name)
	switch {
	case ns == "phone":
		return "phone/calls"
	case ns == "payments" || ns == "premium" || ns == "stars" || ns == "fragment":
		return "premium/payments"
	case ns == "stories":
		return "stories"
	case ns == "stats":
		return "stats"
	case ns == "bots" || strings.HasPrefix(ns, "bot"):
		return "bots"
	case ns == "chatlists" || ns == "folders" || strings.Contains(n, "dialogfilter"):
		return "folders"
	case ns == "langpack":
		return "langpack"
	case ns == "photos":
		return "photos"
	case ns == "upload" || ns == "upload_file":
		return "upload"
	case strings.Contains(n, "business") || ns == "business":
		return "business"
	case ns == "stickers" || strings.Contains(n, "sticker") || strings.HasSuffix(n, "gif") || strings.Contains(n, "savedgif"):
		return "stickers"
	case ns == "contacts":
		return "contacts"
	case ns == "users":
		return "users"
	case ns == "updates":
		return "updates"
	case ns == "help":
		return "help"
	case ns == "channels":
		return "channels"
	case ns == "auth":
		return "auth"
	case ns == "account":
		return "account"
	case ns == "messages":
		return "messages"
	default:
		if ns == "" {
			return "other"
		}
		return ns
	}
}

func phaseOf(module string, startup, background bool) string {
	if startup || background {
		return "P0"
	}
	if p1Modules[module] {
		return "P1"
	}
	return "P2"
}
