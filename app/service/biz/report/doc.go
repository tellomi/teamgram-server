// Package report stores every user report in one MySQL table and, when
// configured, an evidence snapshot under the reports/ prefix.
//
// TELLOMI_REPORT_REVIEW=1 asks pkg/tellomiai to classify a new row.
// An unset value does the same when TELLOMI_AI=1 and TELLOMI_AI_API_KEY
// are set. TELLOMI_REPORT_REVIEW=0 leaves the row for a person.
// TELLOMI_REPORT_HTTP=1 listens for POST /internal/v1/reports,
// POST /internal/v1/reports/subjects, POST /internal/v1/reports/{id}/decision,
// and POST /internal/v1/moderation/review. The listener defaults off.
// TELLOMI_REPORT_SNAPSHOT=1 uploads the evidence object. The @admin
// receipt does not depend on those switches. Login and session code are
// not on this path, and this package does not touch Redis.
// The contract is docs/api/reports.md.
package report
