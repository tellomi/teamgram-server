package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/core"
)

// ContactsExportContactToken
// contacts.exportContactToken#f8654027 = ExportedContactToken;
func (s *Service) ContactsExportContactToken(ctx context.Context, request *mtproto.TLContactsExportContactToken) (*mtproto.ExportedContactToken, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("contacts.exportContactToken - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ContactsExportContactToken(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("contacts.exportContactToken - reply: {%s}", r)
	return r, nil
}

// ContactsImportContactToken
// contacts.importContactToken#13005788 token:string = User;
func (s *Service) ContactsImportContactToken(ctx context.Context, request *mtproto.TLContactsImportContactToken) (*mtproto.User, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("contacts.importContactToken - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ContactsImportContactToken(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("contacts.importContactToken - reply: {%s}", r)
	return r, nil
}

// ContactsExportNoPaidMessagesToken
// contacts.exportNoPaidMessagesToken#83c586ae flags:# revoke:flags.0?true temporary:flags.1?true = ExportedNoPaidMessagesToken;
func (s *Service) ContactsExportNoPaidMessagesToken(ctx context.Context, request *mtproto.TLContactsExportNoPaidMessagesToken) (*mtproto.ExportedNoPaidMessagesToken, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("contacts.exportNoPaidMessagesToken - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ContactsExportNoPaidMessagesToken(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("contacts.exportNoPaidMessagesToken - reply: {%s}", r)
	return r, nil
}

// ContactsImportNoPaidMessagesToken
// contacts.importNoPaidMessagesToken#35a7c6ca user_id:InputUser token:string = User;
func (s *Service) ContactsImportNoPaidMessagesToken(ctx context.Context, request *mtproto.TLContactsImportNoPaidMessagesToken) (*mtproto.User, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("contacts.importNoPaidMessagesToken - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.ContactsImportNoPaidMessagesToken(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("contacts.importNoPaidMessagesToken - reply: {%s}", r)
	return r, nil
}
