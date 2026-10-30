package p1rest

import (
	"regexp"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ConfirmText is the dialog copy messages.checkHistoryImportPeer returns.
const ConfirmText = "Import messages from another app into this chat? People in the chat will be able to see them."

var (
	jsonType = regexp.MustCompile(`"type"\s*:\s*"([a-z_]+)"`)
	jsonName = regexp.MustCompile(`"name"\s*:\s*"([^"\\]*)"`)
	whatsApp = regexp.MustCompile(`(?m)(?:^|\n)\[?\d{1,4}[./-]\d{1,2}[./-]\d{1,4}`)
)

type importJob struct {
	user       int64
	peer       peerRef
	mediaCount int32
	uploaded   int
	started    bool
}

func checkHistory(head string) (*mtproto.Messages_HistoryImportParsed, error) {
	pm, group, title, err := parseHead(head)
	if err != nil {
		return nil, err
	}
	out := &mtproto.Messages_HistoryImportParsed{Pm: pm, Group: group}
	if title != "" {
		out.Title = wrapperspb.String(title)
	}
	return mtproto.MakeTLMessagesHistoryImportParsed(out).To_Messages_HistoryImportParsed(), nil
}

func parseHead(head string) (pm, group bool, title string, err error) {
	s := strings.TrimSpace(head)
	if s == "" {
		return false, false, "", mtproto.ErrImportFormatUnrecognized
	}
	if strings.Contains(s, "\"messages\"") || jsonType.MatchString(s) {
		typ := ""
		if m := jsonType.FindStringSubmatch(s); m != nil {
			typ = m[1]
		}
		if m := jsonName.FindStringSubmatch(s); m != nil {
			title = m[1]
		}
		switch typ {
		case "personal_chat", "saved_messages", "bot_chat":
			return true, false, title, nil
		case "private_group", "public_group", "private_supergroup", "public_supergroup":
			return false, true, title, nil
		case "":
			if strings.Contains(s, "\"messages\"") {
				return false, true, title, nil
			}
		}
		return false, false, "", mtproto.ErrImportFormatUnrecognized
	}
	if whatsApp.MatchString(s) {
		group = strings.Contains(s, " created group ") || strings.Contains(s, " added ")
		return !group, group, "", nil
	}
	return false, false, "", mtproto.ErrImportFormatUnrecognized
}

func (s *store) checkPeer(self int64, peer *mtproto.InputPeer) (*mtproto.Messages_CheckedHistoryImportPeer, error) {
	if _, _, err := peerOf(self, peer); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesCheckedHistoryImportPeer(&mtproto.Messages_CheckedHistoryImportPeer{
		ConfirmText: ConfirmText,
	}).To_Messages_CheckedHistoryImportPeer(), nil
}

func (s *store) initImport(self int64, in *mtproto.TLMessagesInitHistoryImport) (*mtproto.Messages_HistoryImport, error) {
	ref, _, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	file := in.GetFile()
	if file == nil || file.GetId_INT64() == 0 || file.GetParts() <= 0 {
		return nil, mtproto.ErrImportFileInvalid
	}
	switch file.GetPredicateName() {
	case mtproto.Predicate_inputFile, mtproto.Predicate_inputFileBig:
	default:
		return nil, mtproto.ErrImportFileInvalid
	}
	if in.GetMediaCount() < 0 {
		return nil, mtproto.ErrImportFileInvalid
	}
	s.mu.Lock()
	s.importSeq++
	id := s.importSeq
	s.imports[id] = &importJob{user: self, peer: ref, mediaCount: in.GetMediaCount()}
	s.mu.Unlock()
	return mtproto.MakeTLMessagesHistoryImport(&mtproto.Messages_HistoryImport{Id: id}).To_Messages_HistoryImport(), nil
}

func (s *store) uploadImported(self int64, in *mtproto.TLMessagesUploadImportedMedia) (*mtproto.MessageMedia, error) {
	ref, _, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if in.GetFileName() == "" || in.GetMedia() == nil || in.GetMedia().GetPredicateName() == "" || in.GetMedia().GetPredicateName() == mtproto.Predicate_inputMediaEmpty {
		return nil, mtproto.ErrMediaInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.imports[in.GetImportId()]
	if job == nil || job.user != self {
		return nil, mtproto.ErrImportIdInvalid
	}
	if job.peer != ref {
		return nil, mtproto.ErrPeerIdInvalid
	}
	job.uploaded++
	return mtproto.MakeTLMessageMediaEmpty(&mtproto.MessageMedia{}).To_MessageMedia(), nil
}

func (s *store) startImport(self int64, in *mtproto.TLMessagesStartHistoryImport) (*mtproto.Bool, error) {
	ref, _, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.imports[in.GetImportId()]
	if job == nil || job.user != self {
		return nil, mtproto.ErrImportIdInvalid
	}
	if job.peer != ref {
		return nil, mtproto.ErrPeerIdInvalid
	}
	job.started = true
	return mtproto.BoolTrue, nil
}
