package starlarkrule

//import (
//	"path"
//	//"path/filepath"
//
//	"github.com/emcfarlane/larking/starlib/starlarkstruct"
//	"go.starlark.net/starlark"
//)

/*var fileModule = &starlarkstruct.Module{
	Name: "file",
	Members: starlark.StringDict{
		"stat":  starext.MakeBuiltin("file.stat", fileStat),
	},
}*/

//
// workspace(name = "github.com/emcfarlane/string")
//

// File
//const FileConstructor starlark.String = "file"

//func NewFile(rootDir string, fi fs.FileInfo) (*starlarkstruct.Struct, error) {
//	name := fi.Name()
//
//	//dir := path.Dir()
//	//ospath, err := filepath.Abs(filepath.FromSlash(key))
//	//if err != nil {
//	//	return nil, err
//	//}
//	return starlarkstruct.FromStringDict(FileConstructor, starlark.StringDict{
//		"name": starlark.String(name),
//		"base": starlark.String(name),
//		//"dir":          starlark.String(filepath.FromSlash(dir)),
//		"extension": starlark.String(path.Ext(name)),
//		//"path":         starlark.String(ospath),
//		"is_directory": starlark.Bool(fi.IsDir()),
//		//"is_source":    starlark.Bool(isSource),
//		"size": starlark.MakeInt64(fi.Size()),
//	}), nil
//}

//func fileStat(thread *starlark.Thread, fnname string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
//	return NewFile()
//
//}
