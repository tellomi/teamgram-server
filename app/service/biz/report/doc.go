// Package report stores every user report in one MySQL table and, when
// configured, an evidence snapshot under the reports/ prefix.
//
// TELLOMI_REPORT_REVIEW=1 asks pkg/tellomiai to classify a new row. Any
// other value leaves the row for a person. TELLOMI_REPORT_HTTP=1 listens
// for POST /internal/v1/reports, POST /internal/v1/reports/{id}/decision,
// and POST /internal/v1/moderation/review. Both default off.
// TELLOMI_REPORT_SNAPSHOT=1 uploads the evidence object. Login and session
// code are not on this path, and this package does not touch Redis.
package report
