package core

import (
	"github.com/teamgram/proto/mtproto"
)

// PaymentsCanPurchaseStore
// payments.canPurchaseStore#4fdc5ea7 purpose:InputStorePaymentPurpose = Bool;
func (c *PremiumCore) PaymentsCanPurchaseStore(in *mtproto.TLPaymentsCanPurchaseStore) (*mtproto.Bool, error) {
	return mtproto.BoolFalse, nil
}
