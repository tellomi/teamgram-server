package main

import (
	"path/filepath"
	"strings"
)

// whenOf reports whether a call site runs during session startup or in the
// background. A method is P0 when any of its sites is either.
// method is the TL name (namespace.method) so a huge UI file can be limited
// to the RPCs that file issues while logging in.
func whenOf(file, fn, method string) (startup, background bool) {
	f := strings.ToLower(filepath.ToSlash(file))
	if strings.Contains(f, "/test/") || strings.Contains(f, "/tests/") || strings.Contains(f, "benchmark") || strings.Contains(f, "/export/") || strings.Contains(f, "export_api") {
		return false, false
	}
	n := strings.ReplaceAll(strings.ToLower(fn), "_", "")

	type rule struct {
		sub        string
		startup    bool
		background bool
	}
	// File rules are specific bootstrap files. State/ also holds send and
	// history-hole code; those stay interactive so P0 is the session bring-up.
	for _, r := range []rule{
		{"accountstatemanager.swift", true, true},
		{"accountstatemanagementutils.swift", true, true},
		{"unauthorizedaccountstatemanager.swift", true, true},
		{"initializeaccountafterlogin.swift", true, false},
		{"resetstate.swift", true, true},
		{"managedaccountpresence.swift", true, true},
		{"managedconfigurationupdates.swift", true, true},
		{"managedappconfigurationupdates.swift", true, true},
		{"managedlocalizationupdatesoperations.swift", true, true},
		{"synchronizelocalizationupdatesoperation.swift", true, true},
		{"managedpeercolorupdates.swift", true, true},
		{"managedpremiumpromoconfigurationupdates.swift", true, true},
		{"managedproxyinfoupdates.swift", true, true},
		{"managedglobalnotificationsettings.swift", true, true},
		{"managedsynchronizepinnedchatsoperations.swift", true, true},
		{"fetchchatlist.swift", true, false},
		{"availablereactions.swift", true, true},
		{"contactsyncmanager.swift", true, false},
		{"managedautodownloadsettingsupdates.swift", true, true},
		{"telegramengine/localization/", true, true},
		{"telegramengine/auth/", true, false},
		{"registernotificationtoken.swift", true, true},
		{"termsofservice.swift", true, false},
		{"contactmanagement.swift", true, false},
		{"countries.swift", true, false},
		{"authorization.swift", true, false},
		{"config_loader.cpp", true, true},
		{"main_app_config.cpp", true, true},
		{"api_updates.cpp", true, true},
		{"lang_cloud_manager.cpp", true, true},
		{"intro_widget.cpp", true, false},
		{"countries_manager.cpp", true, false},
		{"api_peer_colors.cpp", true, false},
		{"promo_suggestions.cpp", true, false},
		{"api_global_privacy.cpp", true, false},
		{"connectionsmanager.cpp", true, true},
		{"localecontroller.java", true, true},
	} {
		if strings.Contains(f, r.sub) {
			startup = startup || r.startup
			background = background || r.background
		}
	}

	for _, s := range []string{
		"getdifference", "loaddialog", "getdialogs", "fetchchatlist", "loadremotefilters", "getstate", "getconfig",
		"appconfig", "nearestdc", "getlanguage", "langpack", "getcontact",
		"loadcontact", "pinneddialog", "promodata", "termsofservice",
		"peercolor", "getcountries", "dialogfilter", "availablereaction",
		"notifysettings", "globalprivacy", "registerdevice", "updatestatus",
	} {
		if strings.Contains(n, s) {
			startup = true
		}
	}
	for _, s := range []string{
		"getdifference", "updatestatus", "setonline", "presence", "keepalive",
		"registerdevice", "heartbeat",
	} {
		if strings.Contains(n, s) {
			background = true
			startup = true
		}
	}
	lowMethod := strings.ToLower(method)
	if strings.Contains(f, "loginactivity.java") {
		ns, _, _ := strings.Cut(lowMethod, ".")
		switch ns {
		case "auth", "help", "langpack", "account":
			startup = true
		}
	}
	// Folder metadata is loaded with the chat list. Invite management in the
	// same Desktop file is not.
	if (strings.Contains(f, "data_chat_filters.cpp") || strings.Contains(f, "intro_step.cpp")) && strings.Contains(lowMethod, "dialogfilter") {
		startup = true
	}
	if strings.Contains(f, "macrequests.swift") && lowMethod == "help.getappconfig" {
		startup = true
	}
	return startup, background
}

func whenLabel(startup, background bool) string {
	switch {
	case startup && background:
		return "startup+background"
	case startup:
		return "startup"
	case background:
		return "background"
	default:
		return "interactive"
	}
}
