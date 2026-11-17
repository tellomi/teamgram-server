package mtproto

import "context"

// PersonalPhoto is the contact-only photo viewer saved for contact.
// photos.uploadContactProfilePhoto registers it. A nil hook leaves
// user.photo as the public profile photo.
var PersonalPhoto func(ctx context.Context, viewer, contact int64) *Photo

// ApplyPersonalPhoto replaces user.photo with that contact-only photo and
// sets personal. The contact's own profile photo stays on userFull.
// Another viewer is unchanged because the hook returns nil for them.
func ApplyPersonalPhoto(ctx context.Context, viewer int64, user *User) {
	if PersonalPhoto == nil || user == nil || viewer == 0 || user.GetId() == 0 || viewer == user.GetId() {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	profile := PersonalUserProfilePhoto(PersonalPhoto(ctx, viewer, user.GetId()))
	if profile == nil {
		return
	}
	user.Photo = profile
}

// PersonalUserProfilePhoto is userProfilePhoto with personal set.
// A stored photo that has no image sizes still carries photo_id. The chat
// list and the chat header download the file from that id.
func PersonalUserProfilePhoto(photo *Photo) *UserProfilePhoto {
	if photo == nil || photo.GetId() == 0 || photo.GetPredicateName() == Predicate_photoEmpty {
		return nil
	}
	profile := MakeUserProfilePhotoByPhoto(photo)
	if profile == nil || profile.GetPredicateName() == Predicate_userProfilePhotoEmpty {
		profile = MakeTLUserProfilePhoto(&UserProfilePhoto{
			PhotoId: photo.GetId(),
			DcId:    profilePhotoDC(photo),
		}).To_UserProfilePhoto()
	}
	if profile == nil {
		return nil
	}
	profile.Personal = true
	return profile
}
