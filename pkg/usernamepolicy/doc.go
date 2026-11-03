// Package usernamepolicy decides whether a public username may be used.
//
// The check follows the Tellomi policy engine (ADR-0062): NFKC and case
// fold, then exact, underscore-boundary prefix and suffix, and confusable
// skeleton. A denial is USERNAME_OCCUPIED. The rule id is written to the
// server log and is not put on the error.
//
// TELLOMI_USERNAME_POLICY=1 enables the check. Any other value, including
// unset, skips it.
//
// The lexicon embedded in this package is built from requirements v2
// appendix C (terms already in policy-2026092301, plus the D-56 platform
// names and the other "新增" rows). tellomi/tellomi is private and was not
// readable here, so the upstream dist JSON and its 20029/63 corpus are not
// vendored. policy/lexicons/regions/cn does not exist yet (D-25); this
// package does not invent that list. Official appendix D names are not a
// global allow: Reject lets the owning peer through and refuses everyone
// else. Peer id 0 placeholders have no owner.
package usernamepolicy
