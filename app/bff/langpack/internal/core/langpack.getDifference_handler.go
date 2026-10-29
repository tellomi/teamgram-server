package core

import "github.com/teamgram/proto/mtproto"

// LangpackGetDifference
// langpack.getDifference#cd984aa5 lang_pack:string lang_code:string from_version:int = LangPackDifference;
func (c *LangpackCore) LangpackGetDifference(in *mtproto.TLLangpackGetDifference) (*mtproto.LangPackDifference, error) {
	diff, err := c.catalog().Difference(c.resolvePack(in.GetLangPack()), c.resolveLang(in.GetLangCode()), in.GetFromVersion())
	if err != nil {
		return nil, rpcErr(err)
	}
	return diff, nil
}
