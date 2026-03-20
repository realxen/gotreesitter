//go:build !grammar_subset

package grammars

func init() {
	registerTokenSourceFactory("authzed", NewAuthzedTokenSourceOrEOF)
	registerTokenSourceFactory("c", NewCTokenSourceOrEOF)
	registerTokenSourceFactory("cpp", NewCTokenSourceOrEOF)
	registerTokenSourceFactory("go", NewGoTokenSourceOrEOF)
	registerTokenSourceFactory("java", NewJavaTokenSourceOrEOF)
	registerTokenSourceFactory("json", NewJSONTokenSourceOrEOF)
	registerTokenSourceFactory("lua", NewLuaTokenSourceOrEOF)
}
