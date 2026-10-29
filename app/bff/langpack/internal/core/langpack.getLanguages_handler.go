package core

import "github.com/teamgram/proto/mtproto"

// LangpackGetLanguages
// langpack.getLanguages#42c6978f lang_pack:string = Vector<LangPackLanguage>;
func (c *LangpackCore) LangpackGetLanguages(in *mtproto.TLLangpackGetLanguages) (*mtproto.Vector_LangPackLanguage, error) {
	langs, err := c.catalog().Languages(c.resolvePack(in.GetLangPack()))
	if err != nil {
		return nil, rpcErr(err)
	}
	return &mtproto.Vector_LangPackLanguage{Datas: langs}, nil
}
