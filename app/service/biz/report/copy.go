package report

import (
	"strconv"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/langpack"
)

// Copy is the private-message text from user 777000. Chinese says 经平台审核
// (traditional: 經平台審核). No language names a model. Every language
// langpack.getLanguages can list has its own sentences. English is used
// only when the tag is not one of those languages.
func copyLang(code string) string {
	c := langpack.ClientLang(code, "", "")
	if _, ok := reportLines[c]; ok {
		return c
	}
	return "en"
}

func etaText(hours int, lang string) string {
	if hours <= 0 {
		hours = 24
	}
	n := strconv.Itoa(hours)
	line := reportLines[copyLang(lang)]
	if hours == 1 {
		return n + line.hour1
	}
	return n + line.hourN
}

// JoinTickets renders T1、T2 or T1, T2 in id order.
func JoinTickets(ids []int64, lang string) string {
	cp := append([]int64(nil), ids...)
	for i := 1; i < len(cp); i++ {
		j := i
		for j > 0 && cp[j] < cp[j-1] {
			cp[j], cp[j-1] = cp[j-1], cp[j]
			j--
		}
	}
	parts := make([]string, len(cp))
	for i, id := range cp {
		parts[i] = Ticket(id)
	}
	return strings.Join(parts, reportLines[copyLang(lang)].sep)
}

// Receipt is the immediate @admin note to the reporter.
func Receipt(lang, ticket, eta string) string {
	line := reportLines[copyLang(lang)]
	return line.receiptA + ticket + line.receiptB + eta + line.receiptC
}

// DecisionForReporter tells the person who filed the report how it ended.
// tickets is already rendered. A clear result is only for the reporter.
func DecisionForReporter(lang, verdict, tickets string) string {
	line := reportLines[copyLang(lang)]
	if verdict == StatusViolation {
		return line.violA + tickets + line.violB
	}
	return line.clearA + tickets + line.clearB
}

// DecisionForSubject tells the reported person about a violation.
// The text has no reporter id, name, or ticket. A clear result returns empty
// because that person is not notified.
func DecisionForSubject(lang, verdict string) string {
	if verdict != StatusViolation {
		return ""
	}
	return reportLines[copyLang(lang)].subject
}
