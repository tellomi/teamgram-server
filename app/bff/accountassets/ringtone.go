package accountassets

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// GetSavedRingtones implements account.getSavedRingtones.
// https://core.telegram.org/method/account.getSavedRingtones
func GetSavedRingtones(self int64, hash int64) *mtproto.Account_SavedRingtones {
	return shared().savedTones(self, hash)
}

func (s *store) savedTones(self int64, hash int64) *mtproto.Account_SavedRingtones {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[self]
	var rows []*docRec
	if u != nil {
		for id := range u.savedTones {
			rec := s.docs[id]
			if rec == nil {
				continue
			}
			rows = append(rows, rec)
		}
	}
	sortDocs(rows)
	pairs := make([]idVer, 0, len(rows))
	out := make([]*mtproto.Document, 0, len(rows))
	for _, rec := range rows {
		pairs = append(pairs, idVer{id: rec.doc.GetId(), ver: 1})
		out = append(out, cloneDoc(rec.doc))
	}
	sum := hashPairs(pairs)
	if notModified(hash, sum) {
		return mtproto.MakeTLAccountSavedRingtonesNotModified(&mtproto.Account_SavedRingtones{}).To_Account_SavedRingtones()
	}
	return mtproto.MakeTLAccountSavedRingtones(&mtproto.Account_SavedRingtones{
		Hash:      sum,
		Ringtones: out,
	}).To_Account_SavedRingtones()
}

func sortDocs(rows []*docRec) {
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && rows[j].doc.GetId() < rows[j-1].doc.GetId() {
			rows[j], rows[j-1] = rows[j-1], rows[j]
			j--
		}
	}
}

// UploadRingtone implements account.uploadRingtone.
// The file bytes stay with the uploader. The returned document is the id
// account.saveRingtone accepts.
// https://core.telegram.org/method/account.uploadRingtone
func UploadRingtone(self int64, in *mtproto.TLAccountUploadRingtone) (*mtproto.Document, error) {
	return shared().uploadTone(self, in)
}

func (s *store) uploadTone(self int64, in *mtproto.TLAccountUploadRingtone) (*mtproto.Document, error) {
	if in == nil {
		return nil, mtproto.ErrFileIdInvalid
	}
	if err := fileError(in.GetFile()); err != nil {
		return nil, err
	}
	mime := strings.ToLower(strings.TrimSpace(in.GetMimeType()))
	if !ringtoneMIME[mime] {
		return nil, mtproto.ErrRingtoneMimeInvalid
	}
	name := strings.TrimSpace(in.GetFileName())
	if name == "" {
		name = in.GetFile().GetName()
	}
	if name == "" {
		name = "ringtone"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.putDoc(self, "tone", mime, name, in.GetFile().GetParts(), true)
	return cloneDoc(rec.doc), nil
}

// SaveRingtone implements account.saveRingtone.
// An MP3 is saved as account.savedRingtone. Any other accepted type is
// returned as account.savedRingtoneConverted with an audio/mpeg document.
// https://core.telegram.org/method/account.saveRingtone
func SaveRingtone(self int64, in *mtproto.TLAccountSaveRingtone) (*mtproto.Account_SavedRingtone, error) {
	return shared().saveTone(self, in)
}

func (s *store) saveTone(self int64, in *mtproto.TLAccountSaveRingtone) (*mtproto.Account_SavedRingtone, error) {
	if in == nil {
		return nil, mtproto.ErrRingtoneInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, err := s.docFor(self, in.GetId(), "tone")
	if err != nil {
		return nil, mtproto.ErrRingtoneInvalid
	}
	u := s.user(self)
	unsave := mtproto.FromBool(in.GetUnsave())
	target := rec
	if rec.converted != 0 {
		if next := s.docs[rec.converted]; next != nil {
			target = next
		}
	}
	if unsave {
		delete(u.savedTones, rec.doc.GetId())
		delete(u.savedTones, target.doc.GetId())
		return mtproto.MakeTLAccountSavedRingtone(&mtproto.Account_SavedRingtone{}).To_Account_SavedRingtone(), nil
	}
	if !rec.mp3 && rec.converted == 0 {
		name := strings.TrimSuffix(rec.name, extOf(rec.name)) + ".mp3"
		next := s.putDoc(self, "tone", "audio/mpeg", name, 1, true)
		rec.converted = next.doc.GetId()
		u.savedTones[next.doc.GetId()] = struct{}{}
		return mtproto.MakeTLAccountSavedRingtoneConverted(&mtproto.Account_SavedRingtone{
			Document: cloneDoc(next.doc),
		}).To_Account_SavedRingtone(), nil
	}
	u.savedTones[target.doc.GetId()] = struct{}{}
	return mtproto.MakeTLAccountSavedRingtone(&mtproto.Account_SavedRingtone{}).To_Account_SavedRingtone(), nil
}

func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}
