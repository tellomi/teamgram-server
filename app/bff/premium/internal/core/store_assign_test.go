package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"

	"google.golang.org/grpc/status"
)

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func withUser(id int64) *PremiumCore {
	return &PremiumCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: id}}
}

func TestAssignStoreReceipt(t *testing.T) {
	c := withUser(4)
	purpose := mtproto.MakeTLInputStorePaymentPremiumSubscription(&mtproto.InputStorePaymentPurpose{}).To_InputStorePaymentPurpose()
	if _, err := c.PaymentsAssignAppStoreTransaction(nil); codeOf(err) != "RECEIPT_EMPTY" {
		t.Fatal(codeOf(err))
	}
	in := &mtproto.TLPaymentsAssignAppStoreTransaction{Receipt: []byte("receipt"), Purpose: purpose}
	if _, err := c.PaymentsAssignAppStoreTransaction(in); codeOf(err) != "RECEIPT_INVALID" {
		t.Fatal(codeOf(err))
	}
	in.Purpose = nil
	if _, err := c.PaymentsAssignAppStoreTransaction(in); codeOf(err) != "INPUT_PURPOSE_INVALID" {
		t.Fatal(codeOf(err))
	}
	if _, err := (&PremiumCore{}).PaymentsAssignAppStoreTransaction(in); codeOf(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(codeOf(err))
	}
	play := &mtproto.TLPaymentsAssignPlayMarketTransaction{
		Receipt: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"token":"abc"}`}).To_DataJSON(),
		Purpose: purpose,
	}
	if _, err := c.PaymentsAssignPlayMarketTransaction(play); codeOf(err) != "RECEIPT_INVALID" {
		t.Fatal(codeOf(err))
	}
	play.Receipt = mtproto.MakeTLDataJSON(&mtproto.DataJSON{}).To_DataJSON()
	if _, err := c.PaymentsAssignPlayMarketTransaction(play); codeOf(err) != "RECEIPT_EMPTY" {
		t.Fatal(codeOf(err))
	}
	bad := mtproto.MakeTLInputStorePaymentStarsTopup(&mtproto.InputStorePaymentPurpose{
		Stars: 5, Currency: "12", Amount: 10,
	}).To_InputStorePaymentPurpose()
	if err := storePurpose(bad); codeOf(err) != "INPUT_PURPOSE_INVALID" {
		t.Fatal(codeOf(err))
	}
}
