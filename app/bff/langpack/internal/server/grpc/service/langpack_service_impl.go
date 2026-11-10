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
	c.Logger.Debugf("langpack.getLangPack - lang_pack: %s, lang_code: %s", request.GetLangPack(), request.GetLangCode())
	r, err := c.LangpackGetLangPack(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getLangPack - lang_code: %s, version: %d, strings: %d", r.GetLangCode(), r.GetVersion(), len(r.GetStrings()))
	return r, nil
}

// LangpackGetStrings
// langpack.getStrings#efea3803 lang_pack:string lang_code:string keys:Vector<string> = Vector<LangPackString>;
func (s *Service) LangpackGetStrings(ctx context.Context, request *mtproto.TLLangpackGetStrings) (*mtproto.Vector_LangPackString, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getStrings - lang_pack: %s, lang_code: %s, keys: %d", request.GetLangPack(), request.GetLangCode(), len(request.GetKeys()))
	r, err := c.LangpackGetStrings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getStrings - strings: %d", len(r.GetDatas()))
	return r, nil
}

// LangpackGetDifference
// langpack.getDifference#cd984aa5 lang_pack:string lang_code:string from_version:int = LangPackDifference;
func (s *Service) LangpackGetDifference(ctx context.Context, request *mtproto.TLLangpackGetDifference) (*mtproto.LangPackDifference, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getDifference - lang_pack: %s, lang_code: %s, from_version: %d", request.GetLangPack(), request.GetLangCode(), request.GetFromVersion())
	r, err := c.LangpackGetDifference(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getDifference - lang_code: %s, from: %d, version: %d, strings: %d", r.GetLangCode(), r.GetFromVersion(), r.GetVersion(), len(r.GetStrings()))
	return r, nil
}

// LangpackGetLanguages
// langpack.getLanguages#42c6978f lang_pack:string = Vector<LangPackLanguage>;
func (s *Service) LangpackGetLanguages(ctx context.Context, request *mtproto.TLLangpackGetLanguages) (*mtproto.Vector_LangPackLanguage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("langpack.getLanguages - lang_pack: %s", request.GetLangPack())
	r, err := c.LangpackGetLanguages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("langpack.getLanguages - languages: %d", len(r.GetDatas()))
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
