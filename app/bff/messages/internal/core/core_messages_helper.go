// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"
	"math/rand"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/webpage"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/teamgram/teamgram-server/pkg/livephoto"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// outgoingTopicID is the forum topic a send clears when clear_draft is set.
// The official flag is reply_to.top_msg_id. Zero clears the dialog draft only.
func outgoingTopicID(reply *mtproto.InputReplyTo, top *wrapperspb.Int32Value) int32 {
	if top != nil && top.GetValue() > 0 {
		return top.GetValue()
	}
	return dialog.InputTopicID(reply)
}

// draft
func (c *MessagesCore) doClearDraft(ctx context.Context, userId int64, authKeyId int64, peer *mtproto.PeerUtil, top int32) {
	var (
		rV    *mtproto.Bool
		draft *mtproto.DraftMessage
	)
	if top > 0 {
		draft = mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
			Date_FLAGINT32: mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
		}).To_DraftMessage()
		dialog.EnsureTopicReply(draft, top)
		rV, _ = c.svcCtx.Dao.DialogClient.DialogSaveDraftMessage(ctx, &dialog.TLDialogSaveDraftMessage{
			UserId:   userId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
			Message:  draft,
		})
	} else {
		rV, _ = c.svcCtx.Dao.DialogClient.DialogClearDraftMessage(ctx, &dialog.TLDialogClearDraftMessage{
			UserId:   userId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
		})
		draft = mtproto.MakeTLDraftMessageEmpty(nil).To_DraftMessage()
	}

	// ClearDraft
	if mtproto.FromBool(rV) {
		c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(
			ctx,
			&sync.TLSyncUpdatesNotMe{
				UserId:        userId,
				PermAuthKeyId: authKeyId,
				Updates:       mtproto.MakeUpdatesByUpdates(dialog.DraftUpdate(peer.ToPeer(), draft)),
			})
	}
}

func (c *MessagesCore) makeMediaByInputMedia(media *mtproto.InputMedia) (messageMedia *mtproto.MessageMedia, err error) {
	switch media.PredicateName {
	case mtproto.Predicate_inputMediaEmpty:
		// inputMediaEmpty#9664f57f = InputMedia;

		messageMedia = mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia()
	case mtproto.Predicate_inputMediaUploadedPhoto:
		// inputMediaUploadedPhoto#1e287d04 flags:#
		//	file:InputFile
		//	stickers:flags.0?Vector<InputDocument>
		//	ttl_seconds:flags.1?int = InputMedia;

		var (
			photo *mtproto.Photo
		)
		photo, err = c.svcCtx.Dao.MediaClient.MediaUploadPhotoFile(c.ctx, &mediapb.TLMediaUploadPhotoFile{
			OwnerId:    c.MD.PermAuthKeyId,
			File:       media.File,
			Stickers:   media.GetStickers(),
			TtlSeconds: nil,
		})
		if err != nil {
			c.Logger.Errorf("UploadPhoto error: %v, by %s", err, media)
			return
		}

		video, err2 := livephoto.Video(media, c.resolveInputDocument)
		if err2 != nil {
			return nil, err2
		}
		markPhotoStickers(photo, media.GetStickers())
		messageMedia = livephoto.Set(mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Spoiler:         media.Spoiler,
			Photo_FLAGPHOTO: photo,
			TtlSeconds:      media.TtlSeconds,
		}).To_MessageMedia(), video)
	case mtproto.Predicate_inputMediaPhoto:
		// inputMediaPhoto#b3ba0635 flags:#
		//	id:InputPhoto
		//	ttl_seconds:flags.0?int = InputMedia;

		mediaPhoto := media.To_InputMediaPhoto()
		photoIn := mediaPhoto.GetId_INPUTPHOTO()
		photo, err2 := catalog.ResolveInputPhoto(photoIn, func() (*mtproto.Photo, error) {
			if photoIn == nil {
				return nil, mtproto.ErrMediaInvalid
			}
			return c.svcCtx.Dao.MediaClient.MediaGetPhoto(c.ctx, &mediapb.TLMediaGetPhoto{PhotoId: photoIn.GetId()})
		})
		if err2 != nil {
			err = err2
			return
		}

		video, err3 := livephoto.Video(media, c.resolveInputDocument)
		if err3 != nil {
			return nil, err3
		}
		markPhotoStickers(photo, media.GetStickers())
		messageMedia = livephoto.Set(mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
			Spoiler:         mediaPhoto.GetSpoiler(),
			Photo_FLAGPHOTO: photo,
			TtlSeconds:      mediaPhoto.GetTtlSeconds(),
		}).To_MessageMedia(), video)
	case mtproto.Predicate_inputMediaGeoPoint:
		// inputMediaGeoPoint#f9c44144 geo_point:InputGeoPoint = InputMedia;

		messageMedia = mtproto.MakeTLMessageMediaGeo(&mtproto.MessageMedia{
			Geo: mtproto.MakeGeoPointByInput(media.To_InputMediaGeoPoint().GetGeoPoint()),
		}).To_MessageMedia()
	case mtproto.Predicate_inputMediaContact:
		// inputMediaContact#f8ab7dfb
		//	phone_number:string
		//	first_name:string
		//	last_name:string
		//	vcard:string = InputMedia;

		contact := media.To_InputMediaContact()

		messageMedia = mtproto.MakeTLMessageMediaContact(&mtproto.MessageMedia{
			PhoneNumber: contact.GetPhoneNumber(),
			FirstName:   contact.GetFirstName(),
			LastName:    contact.GetLastName(),
			Vcard:       contact.GetVcard(),
			UserId:      0,
		}).To_MessageMedia()

		phoneNumber, err := phonenumber.CheckAndGetPhoneNumber(contact.GetPhoneNumber())
		if err == nil {
			cId, _ := c.svcCtx.Dao.UserClient.UserGetUserIdByPhone(c.ctx, &userpb.TLUserGetUserIdByPhone{
				Phone: phoneNumber,
			})
			if cId != nil {
				messageMedia.UserId = cId.GetV()
			}
		}
	case mtproto.Predicate_inputMediaUploadedDocument:
		// inputMediaUploadedDocument#5b38c6c1 flags:#
		//	nosound_video:flags.3?true
		//	force_file:flags.4?true
		//	file:InputFile
		//	thumb:flags.2?InputFile
		//	mime_type:string
		//	attributes:Vector<DocumentAttribute>
		//	stickers:flags.0?Vector<InputDocument>
		//	ttl_seconds:flags.1?int = InputMedia;
		documentMedia, err2 := c.svcCtx.Dao.MediaClient.MediaUploadedDocumentMedia(c.ctx, &mediapb.TLMediaUploadedDocumentMedia{
			OwnerId: c.MD.PermAuthKeyId,
			Media:   media,
		})
		if err2 != nil {
			if mtproto.IsFilePartMissing(err2) {
				err = err2
			} else {
				err = mtproto.ErrMediaInvalid
			}
			return
		}
		if err = applyDocumentExtras(c, documentMedia, media); err != nil {
			return nil, err
		}
		messageMedia = documentMedia
	case mtproto.Predicate_inputMediaDocument:
		// inputMediaDocument#33473058 flags:#
		//	id:InputDocument
		//	ttl_seconds:flags.0?int
		//	query:flags.1?string = InputMedia;

		id := media.To_InputMediaDocument().GetId_INPUTDOCUMENT()
		document3, err := catalog.ResolveInputDocument(id, func() (*mtproto.Document, error) {
			return c.svcCtx.Dao.MediaClient.MediaGetDocument(c.ctx, &mediapb.TLMediaGetDocument{
				Id: id.GetId(),
			})
		})
		if err != nil {
			return nil, err
		}

		// messageMediaDocument#7c4414d3 flags:# document:flags.0?Document caption:flags.1?string ttl_seconds:flags.2?int = MessageMedia;
		messageMedia = mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Spoiler:  media.To_InputMediaDocument().GetSpoiler(),
			Document: document3,
			// Caption:    media.To_InputMediaDocument().GetCaption(),
			TtlSeconds: media.To_InputMediaDocument().GetTtlSeconds(),
		}).To_MessageMedia()
		if err = applyDocumentExtras(c, messageMedia, media); err != nil {
			return nil, err
		}
	case mtproto.Predicate_inputMediaVenue:
		// inputMediaVenue#c13d1c11
		//	geo_point:InputGeoPoint
		//	title:string
		//	address:string
		//	provider:string
		//	venue_id:string
		//	venue_type:string = InputMedia;
		venue := media.To_InputMediaVenue()

		// messageMediaVenue#2ec0533f geo:GeoPoint title:string address:string provider:string venue_id:string venue_type:string = MessageMedia;
		messageMedia = mtproto.MakeTLMessageMediaVenue(&mtproto.MessageMedia{
			Geo:       mtproto.MakeGeoPointByInput(venue.GetGeoPoint()),
			Title:     venue.GetTitle(),
			Address:   venue.GetAddress(),
			Provider:  venue.GetProvider_STRING(),
			VenueId:   venue.GetVenueId(),
			VenueType: venue.GetVenueType(),
		}).To_MessageMedia()
	case mtproto.Predicate_inputMediaPhotoExternal, mtproto.Predicate_inputMediaDocumentExternal:
		// Remote URLs are not fetched. Accepting them used to store
		// messageMediaUnsupported on both sides.
		return nil, mtproto.ErrMediaInvalid
	case mtproto.Predicate_inputMediaGame:
		// inputMediaGame#d33f43f3 id:InputGame = InputMedia;

		// TODO(@benqi): Not impl inputMediaGame
		messageMedia = mtproto.MakeTLMessageMediaUnsupported(nil).To_MessageMedia()
	case mtproto.Predicate_inputMediaInvoice:
		// inputMediaInvoice#d9799874 flags:# title:string description:string photo:flags.0?InputWebDocument invoice:Invoice payload:bytes provider:string provider_data:DataJSON start_param:flags.1?string = InputMedia;

		// TODO(@benqi): Not impl inputMediaGame
		messageMedia = mtproto.MakeTLMessageMediaUnsupported(nil).To_MessageMedia()
	case mtproto.Predicate_inputMediaGeoLive:
		// inputMediaGeoLive#971fa843 flags:# stopped:flags.0?true geo_point:InputGeoPoint heading:flags.2?int period:flags.1?int proximity_notification_radius:flags.3?int = InputMedia;

		live := media.To_InputMediaGeoLive()
		messageMedia = mtproto.MakeTLMessageMediaGeoLive(&mtproto.MessageMedia{
			Geo:                         mtproto.MakeGeoPointByInput(live.GetGeoPoint()),
			Heading:                     live.GetHeading(),
			Period:                      live.GetPeriod().GetValue(),
			ProximityNotificationRadius: live.GetProximityNotificationRadius(),
		}).To_MessageMedia()
	case mtproto.Predicate_inputMediaPoll:
		// inputMediaPoll#f94e5f1 flags:# poll:Poll correct_answers:flags.0?Vector<bytes> solution:flags.1?string solution_entities:flags.1?Vector<MessageEntity> = InputMedia;
		// https://core.telegram.org/constructor/inputMediaPoll
		messageMedia, err = chatbox.PollInputMedia(c.ctx, c.MD.UserId, media)
	case mtproto.Predicate_inputMediaDice:
		// inputMediaDice#e66fbf7b emoticon:string = InputMedia;

		if media.Emoticon == "🎲" {
			messageMedia = mtproto.MakeTLMessageMediaDice(&mtproto.MessageMedia{
				Value:    rand.Int31()%6 + 1,
				Emoticon: media.Emoticon,
			}).To_MessageMedia()
		} else if media.Emoticon == "🎯" {
			messageMedia = mtproto.MakeTLMessageMediaDice(&mtproto.MessageMedia{
				Value:    rand.Int31()%6 + 1,
				Emoticon: media.Emoticon,
			}).To_MessageMedia()
		} else if media.Emoticon == "🏀" {
			messageMedia = mtproto.MakeTLMessageMediaDice(&mtproto.MessageMedia{
				Value:    rand.Int31()%5 + 1,
				Emoticon: media.Emoticon,
			}).To_MessageMedia()
		} else {
			messageMedia = mtproto.MakeTLMessageMediaDice(&mtproto.MessageMedia{
				Value:    rand.Int31()%6 + 1,
				Emoticon: media.Emoticon,
			}).To_MessageMedia()
		}
	case mtproto.Predicate_inputMediaStory:
		// inputMediaStory#89fdd778 peer:InputPeer id:int = InputMedia;

		messageMedia = mtproto.MakeTLMessageMediaUnsupported(nil).To_MessageMedia()
	case mtproto.Predicate_inputMediaWebPage:
		// inputMediaWebPage#c21b8849 flags:# force_large_media:flags.0?true force_small_media:flags.1?true optional:flags.2?true url:string = InputMedia;
		// https://core.telegram.org/api/link-preview
		// Tellomi flags.3–6 carry a card the sender fetched locally.
		// See docs/api/client-link-preview.md.
		// messageMediaUnsupported#9f84f49e is what the client draws as the
		// paper-plane "unsupported message" bubble.
		messageMedia, err = webpage.Media(c.ctx, media, c.uploadedPhoto)
	case mtproto.Predicate_inputMediaPaidMedia:
		// Paid media is not open. Accepting it used to store messageMediaUnsupported.
		err = mtproto.ErrMediaInvalid
	case mtproto.Predicate_inputMediaTodo:
		// inputMediaTodo#9fc55fde todo:TodoList = InputMedia;
		// Stored and returned as messageMediaToDo#8a53b014 (layer 232).
		messageMedia, err = mtproto.MessageMediaFromTodo(media.GetTodo())
	default:
		err = mtproto.ErrMediaInvalid
	}

	return
}

// markPhotoStickers records that the photo was sent with attached stickers.
func markPhotoStickers(photo *mtproto.Photo, stickers []*mtproto.InputDocument) {
	if photo != nil && len(stickers) > 0 {
		photo.HasStickers = true
	}
}

// applyDocumentExtras copies video_cover and video_timestamp onto the document.
// A cover that cannot be loaded is MEDIA_INVALID instead of a silent drop.
func applyDocumentExtras(c *MessagesCore, messageMedia *mtproto.MessageMedia, media *mtproto.InputMedia) error {
	if messageMedia == nil || media == nil {
		return nil
	}
	if cover := media.GetVideoCover(); cover != nil && cover.GetId() != 0 {
		photo := c.uploadedPhoto(c.ctx, cover)
		if photo == nil {
			return mtproto.ErrMediaInvalid
		}
		messageMedia.VideoCover = photo
	}
	if ts := media.GetVideoTimestamp(); ts != nil {
		messageMedia.VideoTimestamp = ts
	}
	return nil
}

// uploadedPhoto is a photo the sender already stored with messages.uploadMedia.
// A missing id is omitted. The Open Graph text is still unverified.
func (c *MessagesCore) uploadedPhoto(ctx context.Context, in *mtproto.InputPhoto) *mtproto.Photo {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MediaClient == nil || in == nil || in.GetId() == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	photo, err := catalog.ResolveInputPhoto(in, func() (*mtproto.Photo, error) {
		return c.svcCtx.Dao.MediaGetPhoto(ctx, &mediapb.TLMediaGetPhoto{PhotoId: in.GetId()})
	})
	if err != nil || photo == nil || len(photo.GetSizes()) == 0 {
		return nil
	}
	return photo
}

//// TODO(@benqi): mention...
//func (c *MessagesCore) fixMessageEntities(fromId int64, peer *mtproto.PeerUtil, noWebpage bool, message *mtproto.Message, hasBot func() bool) (*mtproto.Message, error) {
//	var (
//		entities mtproto.MessageEntitySlice
//		idxList  []int
//	)
//
//	/*
//		fmt.Println(*update.ChannelPost.Entities)
//		// For each entity
//		for _, e := range *update.ChannelPost.Entities {
//			// Get the whole update Text
//			str := update.ChannelPost.Text
//			// Encode it into utf16
//			utfEncodedString := utf16.Encode([]rune(str))
//			// Decode just the piece of string I need
//			runeString := utf16.Decode(utfEncodedString[e.Offset : e.Offset+e.Length])
//			// Transform []rune into string
//			str = string(runeString)
//			fmt.Println(str)
//		}
//	*/
//	getIdxList := func() []int {
//		if len(idxList) == 0 {
//			idxList = mention.EncodeStringToUTF16Index(message.Message)
//		}
//		return idxList
//	}
//
//	var firstUrl string
//	rIndexes := xurls.Relaxed().FindAllStringIndex(message.Message, -1)
//	if len(rIndexes) > 0 {
//		if len(idxList) == 0 {
//			getIdxList()
//		}
//		for idx, v := range rIndexes {
//			if idx == 0 {
//				firstUrl = message.Message[v[0]:v[1]]
//			}
//			entityUrl := mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{
//				Offset: int32(idxList[v[0]]),
//				Length: int32(idxList[v[1]] - idxList[v[0]]),
//			})
//			entities = append(entities, entityUrl.To_MessageEntity())
//		}
//	}
//
//	if !noWebpage && firstUrl != "" {
//		// TODO(@benqi): disable
//		if c.svcCtx.Plugin != nil {
//			ctx, _ := metadata.RpcMetadataToOutgoing(c.ctx, c.MD)
//			webpage, _ := c.svcCtx.Plugin.GetWebpagePreview(ctx, firstUrl)
//			if webpage != nil {
//				message.Media = mtproto.MakeTLMessageMediaWebPage(&mtproto.MessageMedia{
//					Webpage: webpage,
//				}).To_MessageMedia()
//			}
//		}
//	}
//
//	for _, entity := range message.Entities {
//		switch entity.PredicateName {
//		case mtproto.Predicate_inputMessageEntityMentionName:
//			if entity.GetUserId_INPUTUSER().GetPredicateName() == mtproto.Predicate_inputUserSelf {
//				entity.UserId_INPUTUSER.UserId = fromId
//			}
//
//			if entity.UserId_INPUTUSER.UserId != 0 {
//				// TODO(@benqi): check user_id
//				entityMentionName := mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
//					Offset:       entity.Offset,
//					Length:       entity.Length,
//					UserId_INT64: entity.UserId_INPUTUSER.UserId,
//				})
//
//				entities = append(entities, entityMentionName.To_MessageEntity())
//			}
//			// }
//		default:
//			entities = append(entities, entity)
//		}
//	}
//
//	tags := mention.GetTags('@', message.Message, '(', ')')
//	if len(tags) > 0 {
//		var nameList = make([]string, 0, len(tags))
//		for _, tag := range tags {
//			nameList = append(nameList, tag.Tag)
//		}
//		//names, _ := c.svcCtx.Dao.UsernameClient.UsernameGetListByUsernameList(c.ctx, &username.TLUsernameGetListByUsernameList{
//		//	Names:                nameList,
//		//})
//		// c.Logger.Debugf("nameList: %v", names)
//
//		for _, tag := range tags {
//			if len(idxList) == 0 {
//				getIdxList()
//			}
//			mention2 := mtproto.MakeTLMessageEntityMention(&mtproto.MessageEntity{
//				Offset: int32(idxList[tag.Index]),
//				Length: int32(idxList[tag.Index+len(tag.Tag)+1] - idxList[tag.Index]),
//			}).To_MessageEntity()
//
//			// stole field UserId_5
//			if v, _ := c.svcCtx.Dao.UsernameClient.UsernameResolveUsername(c.ctx, &username.TLUsernameResolveUsername{
//				Username: tag.Tag,
//			}); v != nil {
//				if v.GetPredicateName() == mtproto.Predicate_peerUser {
//					mention2.UserId_INT64 = v.UserId
//				}
//			}
//			//for _, v := range names.Datas {
//			//	//
//			//	if uname, ok := names[tag.Tag]; ok {
//			//		if uname.PeerType == model.PEER_USER {
//			//			mention2.UserId_INT32 = uname.PeerId
//			//		}
//			//	}
//			//}
//			entities = append(entities, mention2)
//			c.Logger.Infof("mention2: %v", mention2)
//		}
//	}
//
//	tags = mention.GetTags('#', message.Message)
//	for _, tag := range tags {
//		if len(idxList) == 0 {
//			getIdxList()
//		}
//		hashtag := mtproto.MakeTLMessageEntityHashtag(&mtproto.MessageEntity{
//			Offset: int32(idxList[tag.Index]),
//			Length: int32(idxList[tag.Index+len(tag.Tag)+1] - idxList[tag.Index]),
//			Url:    "#" + tag.Tag, // NOTE: hack, steal url field
//		}).To_MessageEntity()
//		entities = append(entities, hashtag)
//	}
//
//	if hasBot != nil && hasBot() {
//		tags = mention.GetTags('/', message.Message)
//		for _, tag := range tags {
//			if len(idxList) == 0 {
//				getIdxList()
//			}
//			hashtag := mtproto.MakeTLMessageEntityBotCommand(&mtproto.MessageEntity{
//				Offset: int32(idxList[tag.Index]),
//				Length: int32(idxList[tag.Index+len(tag.Tag)+1] - idxList[tag.Index]),
//			}).To_MessageEntity()
//			entities = append(entities, hashtag)
//		}
//	}
//
//	sort.Sort(entities)
//	message.Entities = entities
//	return message, nil
//}
