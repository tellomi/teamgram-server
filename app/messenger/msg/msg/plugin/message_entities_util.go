// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package plugin

import (
	"context"
	"sort"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/mention"

	"mvdan.cc/xurls/v2"
)

func RemakeMessage(ctx context.Context, plugin MsgPlugin, message *mtproto.Message, fromId int64, noWebpage bool, hasBot func() bool) *mtproto.Message {
	if message == nil {
		return nil
	}

	var firstURL string
	rIndexes := xurls.Relaxed().FindAllStringIndex(message.Message, -1)
	if len(rIndexes) > 0 {
		firstURL = message.Message[rIndexes[0][0]:rIndexes[0][1]]
	}
	if !noWebpage && firstURL != "" && plugin != nil {
		webpage, _ := plugin.GetWebpagePreview(ctx, firstURL)
		if webpage != nil {
			message.Media = mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
				Webpage: webpage,
			}).To_MessageMedia()
		}
	}

	// Client entities win. Invalid ones (length <= 0, out of range) are
	// dropped before the overlap check so they cannot hide a real URL.
	client := mtproto.SanitizeMessageEntities(message.Message, normalizeClientEntities(message.Entities, fromId))

	var auto []*mtproto.MessageEntity
	idxList := mention.EncodeStringToUTF16Index(message.Message)
	auto = appendAuto(auto, client, urlEntities(message.Message, idxList, rIndexes)...)
	auto = appendAuto(auto, client, mentionEntities(ctx, plugin, message.Message, idxList)...)

	indexed := indexText(message.Message)
	auto = appendAuto(auto, client, findHashtags(indexed)...)
	auto = appendAuto(auto, client, findCashtags(indexed)...)
	if hasBot != nil && hasBot() {
		auto = appendAuto(auto, client, findBotCommands(indexed)...)
	}

	entities := make(mtproto.MessageEntitySlice, 0, len(client)+len(auto))
	entities = append(entities, client...)
	entities = append(entities, auto...)
	sort.Sort(entities)
	message.Entities = entities
	message.SanitizeEntities()
	return message
}

func normalizeClientEntities(entities []*mtproto.MessageEntity, fromID int64) []*mtproto.MessageEntity {
	if len(entities) == 0 {
		return nil
	}
	out := make([]*mtproto.MessageEntity, 0, len(entities))
	for _, entity := range entities {
		if entity == nil {
			continue
		}
		if entity.GetPredicateName() != mtproto.Predicate_inputMessageEntityMentionName {
			out = append(out, entity)
			continue
		}
		inputUser := entity.GetUserId_INPUTUSER()
		if inputUser.GetPredicateName() == mtproto.Predicate_inputUserSelf && inputUser != nil {
			inputUser.UserId = fromID
		}
		if inputUser.GetUserId() == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
			Offset:       entity.Offset,
			Length:       entity.Length,
			UserId_INT64: inputUser.GetUserId(),
		}).To_MessageEntity())
	}
	return out
}

// appendAuto keeps server-detected entities that do not overlap a client
// entity and that cover at least one UTF-16 code unit.
func appendAuto(dst, client []*mtproto.MessageEntity, detected ...*mtproto.MessageEntity) []*mtproto.MessageEntity {
	for _, entity := range detected {
		if entity == nil || entity.Length <= 0 {
			continue
		}
		if overlapsClient(entity, client) {
			continue
		}
		dst = append(dst, entity)
	}
	return dst
}

func overlapsClient(entity *mtproto.MessageEntity, client []*mtproto.MessageEntity) bool {
	for _, other := range client {
		if mtproto.EntityOverlaps(entity, other) {
			return true
		}
	}
	return false
}

func urlEntities(text string, idxList []int, spans [][]int) []*mtproto.MessageEntity {
	var out []*mtproto.MessageEntity
	for _, span := range spans {
		if len(span) != 2 {
			continue
		}
		off, length, ok := utf16Span(idxList, span[0], span[1])
		if !ok || length <= 0 {
			continue
		}
		out = append(out, mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{
			Offset: off,
			Length: length,
		}).To_MessageEntity())
	}
	return out
}

func mentionEntities(ctx context.Context, plugin MsgPlugin, text string, idxList []int) []*mtproto.MessageEntity {
	tags := mention.GetTags('@', text, '(', ')')
	var out []*mtproto.MessageEntity
	for _, tag := range tags {
		if tag.Tag == "" {
			continue
		}
		end := tag.Index + len(tag.Tag) + 1
		off, length, ok := utf16Span(idxList, tag.Index, end)
		if !ok || length <= 0 {
			continue
		}
		entity := mtproto.MakeTLMessageEntityMention(&mtproto.MessageEntity{
			Offset: off,
			Length: length,
		}).To_MessageEntity()
		if plugin != nil {
			if v, _ := plugin.UsernameResolveUsername(ctx, &user.TLUserResolveUsername{
				Username: tag.Tag,
			}); v != nil {
				if v.GetPredicateName() == mtproto.Predicate_peerUser {
					entity.UserId_INT64 = v.UserId
				}
			}
		}
		out = append(out, entity)
	}
	return out
}

// utf16Span maps a byte range [start, end) onto UTF-16 offset and length.
// A non-positive length is rejected so callers never build an empty entity.
func utf16Span(idxList []int, start, end int) (int32, int32, bool) {
	if start < 0 || end < start || end >= len(idxList) || start >= len(idxList) {
		return 0, 0, false
	}
	off := idxList[start]
	limit := idxList[end]
	if limit <= off {
		return 0, 0, false
	}
	return int32(off), int32(limit - off), true
}
