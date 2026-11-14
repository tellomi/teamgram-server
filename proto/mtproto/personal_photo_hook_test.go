package mtproto

import (
	"context"
	"testing"
)

func TestPersonalUserProfilePhotoSetsPersonal(t *testing.T) {
	photo := MakeTLPhoto(&Photo{
		Id:    44,
		DcId:  2,
		Sizes: []*PhotoSize{{PredicateName: Predicate_photoSize, Type: "a"}},
	}).To_Photo()
	got := PersonalUserProfilePhoto(photo)
	if got.GetPhotoId() != 44 || !got.GetPersonal() || got.GetDcId() != 2 {
		t.Fatalf("%+v", got)
	}
	bare := PersonalUserProfilePhoto(MakeTLPhoto(&Photo{Id: 44, DcId: 1}).To_Photo())
	if bare.GetPhotoId() != 44 || !bare.GetPersonal() {
		t.Fatalf("bare %+v", bare)
	}
	if PersonalUserProfilePhoto(nil) != nil || PersonalUserProfilePhoto(MakeTLPhotoEmpty(&Photo{Id: 1}).To_Photo()) != nil {
		t.Fatal("empty photo became a profile photo")
	}
}

func TestApplyPersonalPhotoOnlyForTheSetter(t *testing.T) {
	prev := PersonalPhoto
	t.Cleanup(func() { PersonalPhoto = prev })
	PersonalPhoto = func(_ context.Context, viewer, contact int64) *Photo {
		if viewer == 3 && contact == 2 {
			return MakeTLPhoto(&Photo{Id: 44, DcId: 1}).To_Photo()
		}
		return nil
	}
	user := MakeTLUser(&User{Id: 2}).To_User()
	ApplyPersonalPhoto(context.Background(), 3, user)
	if user.GetPhoto().GetPhotoId() != 44 || !user.GetPhoto().GetPersonal() {
		t.Fatalf("%+v", user.GetPhoto())
	}
	other := MakeTLUser(&User{Id: 2}).To_User()
	ApplyPersonalPhoto(context.Background(), 9, other)
	if other.GetPhoto() != nil {
		t.Fatalf("third party %+v", other.GetPhoto())
	}
	self := MakeTLUser(&User{Id: 3}).To_User()
	ApplyPersonalPhoto(context.Background(), 3, self)
	if self.GetPhoto() != nil {
		t.Fatal("self photo was replaced")
	}
}
