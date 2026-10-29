// Package linkpreview is the public read-only HTTP API that resolves
// tell.cc / t.me paths for the Cloudflare Worker preview pages.
//
// Only data that is already public is returned. Users and chats with no
// username, private channels, non-public stories, and revoked invites are
// not_found. Phone numbers, bot tokens, and access hashes are never selected.
package linkpreview
