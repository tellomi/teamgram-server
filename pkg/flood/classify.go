package flood

import "strings"

const (
	classOther    = ""
	classSend     = "send"
	classSendCode = "send_code"
	classSignIn   = "sign_in"
	classImport   = "import"
	classResolve  = "resolve"
	classSearch   = "search"
	classUpload   = "upload"
	classStatus   = "status"
	classAccount  = "account"
	classReport   = "report"
)

// classOf maps a predicate (messages_sendMessage) to a bucket class.
// Reads, including account.get* and updates.getState, stay on the global
// bucket only.
func classOf(method string) string {
	name := strings.TrimSpace(method)
	switch {
	case name == "auth_sendCode" || name == "auth_resendCode" ||
		name == "account_sendChangePhoneCode" || name == "account_sendConfirmPhoneCode":
		return classSendCode
	case name == "auth_signIn" || name == "auth_signUp":
		return classSignIn
	case strings.HasPrefix(name, "messages_send") || name == "messages_forwardMessages" ||
		name == "stories_sendStory" || name == "stories_sendReaction":
		return classSend
	case name == "contacts_importContacts":
		return classImport
	case name == "contacts_resolveUsername" || name == "contacts_resolvePhone":
		return classResolve
	case name == "contacts_search":
		return classSearch
	case name == "upload_saveFilePart" || name == "upload_saveBigFilePart":
		return classUpload
	case name == "account_updateStatus":
		return classStatus
	case strings.HasPrefix(name, "account_update") || strings.HasPrefix(name, "account_set") ||
		strings.HasPrefix(name, "account_delete") || strings.HasPrefix(name, "account_reset") ||
		name == "account_changePhone" || name == "account_registerDevice" || name == "account_unregisterDevice":
		return classAccount
	case strings.Contains(name, "report") || strings.Contains(name, "Report"):
		return classReport
	default:
		return classOther
	}
}
