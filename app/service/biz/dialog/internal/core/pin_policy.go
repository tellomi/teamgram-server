package core

import "github.com/teamgram/proto/mtproto"

// Limits advertised by help.getConfig: pinned_dialogs_count_max and
// pinned_infolder_count_max in teamgramd/bin/config.json. A pin past the
// limit is PINNED_DIALOGS_TOO_MUCH. Clients then refetch and retry
// messages.reorderPinnedDialogs with force.
const (
	pinnedDialogsMax       = 5
	pinnedFolderDialogsMax = 100
)

func pinnedDialogLimit(folderID int32) int {
	if folderID == 1 {
		return pinnedFolderDialogsMax
	}
	return pinnedDialogsMax
}

// reorderPinsRejected is messages.reorderPinnedDialogs without force when
// the server still has a pin the client did not send. force unpins those.
// An order longer than the folder limit is the same error either way.
func reorderPinsRejected(force bool, current, order []int64, limit int) error {
	if limit >= 0 && len(order) > limit {
		return mtproto.ErrPinnedDialogsTooMuch
	}
	if force {
		return nil
	}
	have := make(map[int64]struct{}, len(order))
	for _, id := range order {
		have[id] = struct{}{}
	}
	for _, id := range current {
		if _, ok := have[id]; !ok {
			return mtproto.ErrPinnedDialogsTooMuch
		}
	}
	return nil
}
