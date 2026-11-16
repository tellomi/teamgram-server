// Package usernamepolicy decides whether a public username may be used.
//
// The check follows the engine that built the tellomi/tellomi dist files
// (ADR-0062): reject Cc/Cf, NFKC, Unicode lowercase with ß→ss, collapse
// whitespace, then exact match and prefix/suffix only at a
// non-alphanumeric boundary. CONFUSABLE compares UTS #39 skeletons plus
// the ASCII lookalike fold (the stored tellomi skeleton is telloml). A
// username denial is USERNAME_OCCUPIED. A display-name denial is
// FIRSTNAME_INVALID; the last-name caller maps that to LASTNAME_INVALID.
// Bios, group titles, and group descriptions use the same display-name
// denial. The rule id is written to the server log and is not put on the
// error.
//
// Runtime loads policy/policy-2026110602.json, byte-for-byte the
// tellomi/tellomi dist file (1024 rules, including regions/cn and the
// appendix C / D-56 names). policy/policy-2026092301.json stays embedded
// so the old 470-rule file still loads; the corpus that is executed is
// the 2026110602 one. policy/overlay-d56.json still feeds appendix C
// placeholder SQL. It is not merged into the runtime lexicon. The
// cn username supplement in cn_username_supplement.go is applied beside
// that JSON. It folds latin cn username terms whose skeletons are at
// least 8, from that file and from the runtime MIT-file rules, and
// matches a digit-stripped skeleton at either end. A term buried in the
// middle still passes. pkg/usernamepolicy/lexiconsrc vendors the pinned
// konsheng MIT and fwwdn Apache-2.0 category files. The 391 regions/cn
// rules already in the JSON stay the D-25 admit set. MIT lines that also
// occur in the excluded Tencent file are loaded at runtime unless a drop
// list keeps them out. The Tencent file itself is not loaded. Chat
// message bodies are not passed to this package.
// Existing rows are not rewritten.
//
// TELLOMI_USERNAME_POLICY=1 enables the check. Any other value, including
// unset, skips it. docker-compose.yaml defaults the variable to empty.
// An empty default is not a statement about the production environment:
// this repository does not read production and does not turn the flag on.
package usernamepolicy
