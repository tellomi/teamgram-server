// Package enforcement is the platform account sanction record.
//
// The subject is a user id. Phone and email are login identifiers bound to
// that user; they are not a second sanction. Group bans, contact blocks,
// flood waits, verification quotas, username policy, and self-deletion are
// not records in this package. The 7696-term candidate list is not read.
//
// TELLOMI_ENFORCEMENT_CREATE defaults off. With it off, Create writes
// nothing. Records that already exist are still enforced. There is no switch
// that deletes them.
//
// Notices are private messages from user 777000 (@admin), the seeded service
// account. Appeals are decided by user 777001 (@tellomi) through a Judge.
// An allowlisted admin lifts one active sanction in that same private chat,
// after a second message that is exactly 「是」. That path does not listen.
// Tests supply a fake Judge. That is not a live model call. A model error,
// timeout, or insufficient evidence leaves the appeal pending and does not
// add a sanction.
package enforcement
