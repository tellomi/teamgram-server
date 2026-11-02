-- Shared media tabs for polls, locations and contacts.
-- New messages get message_filter_type 14 (poll), 15 (geo/venue/live
-- location) and 16 (contact) from mtproto.GetMediaType. Backfill rows stored
-- before that as -1. Rows stored as 0 without any media came from an older
-- insert path and made text messages show up under Photos & Videos.
-- Safe to run more than once.

UPDATE `messages` SET `message_filter_type` = 14
WHERE `message_filter_type` = -1
  AND JSON_UNQUOTE(JSON_EXTRACT(`message_data`, '$.media.predicate_name')) = 'messageMediaPoll';

UPDATE `messages` SET `message_filter_type` = 15
WHERE `message_filter_type` = -1
  AND JSON_UNQUOTE(JSON_EXTRACT(`message_data`, '$.media.predicate_name')) IN ('messageMediaGeo', 'messageMediaGeoLive', 'messageMediaVenue');

UPDATE `messages` SET `message_filter_type` = 16
WHERE `message_filter_type` = -1
  AND JSON_UNQUOTE(JSON_EXTRACT(`message_data`, '$.media.predicate_name')) = 'messageMediaContact';

UPDATE `messages` SET `message_filter_type` = -1
WHERE `message_filter_type` = 0
  AND JSON_EXTRACT(`message_data`, '$.media') IS NULL;
