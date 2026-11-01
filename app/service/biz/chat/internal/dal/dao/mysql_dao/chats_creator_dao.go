package mysql_dao

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateCreator moves chats.creator_user_id. version bumps so clients
// refresh the participant list.
func (dao *ChatsDAO) UpdateCreator(ctx context.Context, creatorUserID, id int64) (int64, error) {
	res, err := dao.db.Exec(ctx, "update chats set creator_user_id = ?, version = version + 1 where id = ?", creatorUserID, id)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in UpdateCreator(_), error: %v", err)
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in UpdateCreator(_), error: %v", err)
	}
	return n, err
}

// UpdateCreatorTx is UpdateCreator inside a transaction.
func (dao *ChatsDAO) UpdateCreatorTx(tx *sqlx.Tx, creatorUserID, id int64) (int64, error) {
	res, err := tx.Exec("update chats set creator_user_id = ?, version = version + 1 where id = ?", creatorUserID, id)
	if err != nil {
		logx.WithContext(tx.Context()).Errorf("exec in UpdateCreatorTx(_), error: %v", err)
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		logx.WithContext(tx.Context()).Errorf("rowsAffected in UpdateCreatorTx(_), error: %v", err)
	}
	return n, err
}
