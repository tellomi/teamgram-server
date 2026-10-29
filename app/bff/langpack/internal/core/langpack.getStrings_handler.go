package core

import "github.com/teamgram/proto/mtproto"

// LangpackGetStrings
// langpack.getStrings#efea3803 lang_pack:string lang_code:string keys:Vector<string> = Vector<LangPackString>;
func (c *LangpackCore) LangpackGetStrings(in *mtproto.TLLangpackGetStrings) (*mtproto.Vector_LangPackString, error) {
	strings, err := c.catalog().Strings(c.resolvePack(in.GetLangPack()), c.resolveLang(in.GetLangCode()), in.GetKeys())
	if err != nil {
		return nil, rpcErr(err)
	}
	return &mtproto.Vector_LangPackString{Datas: strings}, nil
}
