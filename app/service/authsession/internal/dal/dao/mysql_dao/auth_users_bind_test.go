package mysql_dao

import "testing"

func TestAuthUserBindUpdateKeepsALiveCreatedTime(t *testing.T) {
	if authUserBindUpdate != "hash = if(deleted = 0 and hash <> 0, hash, values(hash)), date_active = values(date_active), date_created = if(date_created > 0, date_created, values(date_created)), deleted = 0" {
		t.Fatalf("bind update = %s", authUserBindUpdate)
	}
}
