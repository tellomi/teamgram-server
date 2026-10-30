# Feed

`feed.*` is not in the layer-232 schema. iOS and macOS already send the
layer-233 constructors (`feed.getFeed#7e76d7d3` and the rest of that
namespace). They are registered on `RPCFeed`.

There is no channel index, so message pages are empty. Interest selections
and the read cursor are stored in this process. A restart drops them.

`feed.readFeed` returns `updates` with seq 0 and an empty update list. The
cursor is kept for `feed.getFeed` when `offset_to_max_read` is set. The
difference cursor does not move.

## Schema-valid results

- `feed.getInterestCategories` returns `feed.interestCategories` with eight
  categories. A hash this process issued is not echoed in the result, so
  clients that hash the list themselves receive the list again.
- `feed.setInterestCategories` returns `boolTrue`. Duplicate ids are stored
  once. An empty vector clears the selection.
- `feed.getForYou` returns `feed.forYouFeed`. With no selection the only item
  is `feedItemInterestCategories`. After a selection the item list is empty.
  `query` is accepted and does not filter, because there are no posts.
- `feed.getFeed` returns `feed.feedMessages` with empty `messages`, `chats`,
  and `users`. A stored read cursor is `read_max_position`. `limit` 0 is
  accepted. `add_offset` is accepted and does not change the empty page.
- `feed.readFeed` returns the empty updates box above.

## Documented 400s

| Code | When |
| --- | --- |
| `FILTER_ID_INVALID` | `filter_id` is negative |
| `LIMIT_INVALID` | `limit` is negative or greater than 100 |
| `INTEREST_CATEGORY_INVALID` | a category id is not in the catalog |
| `PEER_ID_INVALID` | a position is missing when required, or its peer is empty |
| `MESSAGE_ID_INVALID` | a position `id` is not positive |

`AUTH_KEY_UNREGISTERED` (401) is returned when the RPC has no user id.
