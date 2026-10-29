package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/langpack/internal/core"
)

// LangpackGetLangPack
// langpack.getLangPack#f2f2330a lang_pack:string lang_code:string = LangPackDifference;
func (s *Service) LangpackGetLangPack(ctx context.Context, request *mtproto.TLLangpackGetLangPack) (*mtproto.LangPackDifference, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getLangPack - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.LangpackGetLangPack(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getLangPack - reply: {%s}", r)
	return r, nil
}

// LangpackGetStrings
// langpack.getStrings#efea3803 lang_pack:string lang_code:string keys:Vector<string> = Vector<LangPackString>;
func (s *Service) LangpackGetStrings(ctx context.Context, request *mtproto.TLLangpackGetStrings) (*mtproto.Vector_LangPackString, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getStrings - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.LangpackGetStrings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getStrings - reply: {%s}", r)
	return r, nil
}

// LangpackGetDifference
// langpack.getDifference#cd984aa5 lang_pack:string lang_code:string from_version:int = LangPackDifference;
func (s *Service) LangpackGetDifference(ctx context.Context, request *mtproto.TLLangpackGetDifference) (*mtproto.LangPackDifference, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getDifference - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.LangpackGetDifference(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getDifference - reply: {%s}", r)
	return r, nil
}

// LangpackGetLanguages
// langpack.getLanguages#42c6978f lang_pack:string = Vector<LangPackLanguage>;
func (s *Service) LangpackGetLanguages(ctx context.Context, request *mtproto.TLLangpackGetLanguages) (*mtproto.Vector_LangPackLanguage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getLanguages - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.LangpackGetLanguages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getLanguages - reply: {%s}", r)
	return r, nil
}

// LangpackGetLanguage
// langpack.getLanguage#6a596502 lang_pack:string lang_code:string = LangPackLanguage;
func (s *Service) LangpackGetLanguage(ctx context.Context, request *mtproto.TLLangpackGetLanguage) (*mtproto.LangPackLanguage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getLanguage - metadata: {%s}, request: {%s}", c.MD, request)
	r, err := c.LangpackGetLanguage(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getLanguage - reply: {%s}", r)
	return r, nil
}
