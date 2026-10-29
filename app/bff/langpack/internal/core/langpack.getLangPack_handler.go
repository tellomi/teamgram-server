package core

import "github.com/teamgram/proto/mtproto"

// LangpackGetLangPack
// langpack.getLangPack#f2f2330a lang_pack:string lang_code:string = LangPackDifference;
func (c *LangpackCore) LangpackGetLangPack(in *mtproto.TLLangpackGetLangPack) (*mtproto.LangPackDifference, error) {
	diff, err := c.catalog().Difference(c.resolvePack(in.GetLangPack()), c.resolveLang(in.GetLangCode()), 0)
	if err != nil {
		return nil, rpcErr(err)
	}
	return diff, nil
}
