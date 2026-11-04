// Package contentsafety calls Aliyun Green text moderation for display names.
//
// TELLOMI_CONTENT_SAFETY selects the API. Unset, 0, and off do nothing.
// 1 or aliyun calls TextModeration (nickname_detection) on the Chengdu
// Green endpoint. bailian calls TextModerationPlus (nickname_detection_pro),
// the text API Bailian's guardrail product uses, on the Shanghai endpoint.
// TELLOMI_CONTENT_SAFETY_ENDPOINT and TELLOMI_CONTENT_SAFETY_SERVICE override
// those defaults.
//
// Only text that contains Han characters is sent. ASCII stays on the local
// username lexicon. A block is a positive moderation result. A timeout, an
// HTTP error, a missing access key, or any response that is not a decision
// is a fallback: the name is allowed and the failure is logged. The log has
// the request id and the label, not the name and not the access key.
//
// Credentials are TELLOMI_CONTENT_SAFETY_ACCESS_KEY_ID and
// TELLOMI_CONTENT_SAFETY_ACCESS_KEY_SECRET, or the existing
// ALIBABA_CLOUD_ACCESS_KEY_ID and ALIBABA_CLOUD_ACCESS_KEY_SECRET.
package contentsafety
