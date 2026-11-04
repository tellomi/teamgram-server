// Package usernamepolicy decides whether a public username may be used.
//
// The check follows the engine that built policy-2026092301 (ADR-0062):
// reject Cc/Cf, NFKC, Unicode lowercase with ß→ss, collapse whitespace,
// then exact match and prefix/suffix only at a non-alphanumeric boundary.
// CONFUSABLE compares UTS #39 skeletons plus the ASCII lookalike fold
// (the stored tellomi skeleton is telloml). A username denial is
// USERNAME_OCCUPIED. A display-name denial is FIRSTNAME_INVALID; the
// last-name caller maps that to LASTNAME_INVALID. Bios, group titles, and
// group descriptions use the same display-name denial. The rule id is
// written to the server log and is not put on the error.
//
// The embedded policy/policy-2026092301.json file is the tellomi/tellomi
// dist file, unchanged. Names from requirements appendix C / D-56 that it
// does not already reserve are policy/overlay-d56.json. The corpus test
// loads the official file alone.
//
// TELLOMI_USERNAME_POLICY=1 enables the check. Any other value, including
// unset, skips it.
package usernamepolicy
