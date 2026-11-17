package core

import "testing"

func TestPlanUsernameClaimReleasesThePreviousName(t *testing.T) {
	free := planUsernameClaim(false, false, "", "newname")
	if free.Occupied || !free.RenameEditable || !free.InsertIfMissing {
		t.Fatalf("free name %#v", free)
	}
	taken := planUsernameClaim(true, false, "newname", "newname")
	if !taken.Occupied || taken.RenameEditable || taken.InsertIfMissing || taken.RewriteSpelling {
		t.Fatalf("taken name %#v", taken)
	}
	same := planUsernameClaim(true, true, "newname", "newname")
	if same.Occupied || same.RenameEditable || same.InsertIfMissing || same.RewriteSpelling {
		t.Fatalf("already ours %#v", same)
	}
	spelling := planUsernameClaim(true, true, "NewName", "newname")
	if spelling.Occupied || spelling.RenameEditable || spelling.InsertIfMissing || !spelling.RewriteSpelling {
		t.Fatalf("case change %#v", spelling)
	}
}
