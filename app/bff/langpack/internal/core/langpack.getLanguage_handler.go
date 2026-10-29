package core

import "github.com/teamgram/proto/mtproto"

// LangpackGetLanguage
// langpack.getLanguage#6a596502 lang_pack:string lang_code:string = LangPackLanguage;
func (c *LangpackCore) LangpackGetLanguage(in *mtproto.TLLangpackGetLanguage) (*mtproto.LangPackLanguage, error) {
	lang, err := c.catalog().Language(c.resolvePack(in.GetLangPack()), c.resolveLang(in.GetLangCode()))
	if err != nil {
		return nil, rpcErr(err)
	}
	return lang, nil
}
