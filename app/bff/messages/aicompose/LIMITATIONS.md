# AI compose tones

`aicompose.*` is served on `RPCAiComposeTone`, the same service as
`messages.composeMessageWithAI`. Tones live in this process. A restart drops
custom tones.

New users start with four built-in tones saved: `friendly`, `professional`,
`concise`, and `witty`. Those are `aiComposeToneDefault`. They can be
unsaved. They cannot be edited or deleted.

`aicompose.getToneExample` returns a fixed English template. There is no
composer model. `messages.composeMessageWithAI` is unchanged.

`aicompose.getTones` returns `aicompose.tonesNotModified` when the caller's
hash matches. Creating, saving, unsaving, or deleting a tone does not push
`updateAiComposeTones`. The RPC result is the new tone or `boolTrue`. Other
sessions see the change on the next `aicompose.getTones`. That result has no
pts, so the difference cursor does not move.

## Schema-valid results

- `aicompose.createTone` returns `aiComposeTone` and saves it for the caller.
  `display_author` sets `author_id`. The creator is the only viewer who
  receives `prompt`.
- `aicompose.updateTone` changes only the fields whose flag is set. The slug
  stays the same.
- `aicompose.saveTone` returns `boolTrue`. `unsave` removes the tone from
  the saved list. Saving another user's tone by id or slug installs it.
- `aicompose.deleteTone` returns `boolTrue` and removes the tone from every
  saved list in this process.
- `aicompose.getTone` returns `aicompose.tones` with that one tone. A
  displayed author is `userEmpty`.
- `aicompose.getTones` returns `aicompose.tones` or `aicompose.tonesNotModified`.
- `aicompose.getToneExample` returns `aiComposeToneExample`. `num` selects
  one of three templates.

## Documented 400s

| Code | When |
| --- | --- |
| `AICOMPOSE_TONE_TITLE_INVALID` | title is empty or longer than 64 characters |
| `AICOMPOSE_TONE_INVALID` | prompt is empty or longer than 1024 characters; the tone id, access hash, slug, or built-in name does not match; a built-in or someone else's tone is updated or deleted; `num` is negative |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.
