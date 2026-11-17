package dao

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

func TestContactDataCopiesStoriesHidden(t *testing.T) {
	row := contactDataFromDO(&dataobject.UserContactsDO{
		OwnerUserId:   7,
		ContactUserId: 9,
		CloseFriend:   true,
		StoriesHidden: true,
	})
	if row == nil || !row.GetStoriesHidden() || !row.GetCloseFriend() {
		t.Fatalf("contact %#v", row)
	}
	if contactDataFromDO(nil) != nil {
		t.Fatal("nil contact")
	}
}
