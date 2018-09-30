package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var g_E = flag.Bool("E", false, "生成未fmt前的代码便于排错")
var g_add_servant = flag.Bool("add-servant", true, "生成AddServant函数")

type GenGo struct {
	code     bytes.Buffer
	vc       int      // var count. 用于产生唯一变量名
	I        []string // imports with path
	path     string
	prefix   string
	tarsPath string
	p        *Parse
}

//首字母大写
func upperFirstLatter(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) == 1 {
		return strings.ToUpper(string(s[0]))
	}
	return strings.ToUpper(string(s[0])) + s[1:]
}

// === rename area ===
// 0. rename module
func (p *Parse) rename() {
	p.OriginModule = p.Module
	p.Module = upperFirstLatter(p.Module)
}

// 1. struct rename
// struct TName { 1 require Mb type}
func (st *StructInfo) rename() {
	st.TName = upperFirstLatter(st.TName)
	for i, _ := range st.Mb {
		st.Mb[i].KeyStr = st.Mb[i].Key
		// 成员变量强制首字母大写
		st.Mb[i].Key = upperFirstLatter(st.Mb[i].Key)
	}
}

// 1. interface rename
// interface TName { Fun }
func (itf *InterfaceInfo) rename() {
	itf.TName = upperFirstLatter(itf.TName)
	for i, _ := range itf.Fun {
		itf.Fun[i].rename()
	}
}

func (en *EnumInfo) rename() {
	en.TName = upperFirstLatter(en.TName)
	for i, _ := range en.Mb {
		en.Mb[i].Key = upperFirstLatter(en.Mb[i].Key)
	}
}

func (cst *ConstInfo) rename() {
	cst.Key = upperFirstLatter(cst.Key)
}

// 2. func rename
// type Fun (arg ArgType), argname 本来不用大写，但是为了防止name=关键字的，
// Fun (type int32)
func (fun *FunInfo) rename() {
	fun.NameStr = fun.Name
	fun.Name = upperFirstLatter(fun.Name)
	for i, _ := range fun.Args {
		fun.Args[i].Name = upperFirstLatter(fun.Args[i].Name)
	}
}

// 3. genType 为所有的Type进行rename

// === rename end ===

func (gen *GenGo) genErr(err string) {
	panic(err)
}

func (gen *GenGo) saveToSourceFile(filename string) {
	var beauty []byte
	var err error
	prefix := gen.prefix

	if !*g_E {
		beauty, err = format.Source(gen.code.Bytes())
		if err != nil {
			gen.genErr("go fmt fail. " + err.Error())
		}
	} else {
		beauty = gen.code.Bytes()
	}

	if filename == "stdout" {
		fmt.Println(string(beauty))
	} else {
		err = os.MkdirAll(prefix+gen.p.Module, 0766)

		if err != nil {
			gen.genErr(err.Error())
		}
		err = ioutil.WriteFile(prefix+gen.p.Module+"/"+filename, beauty, 0666)

		if err != nil {
			gen.genErr(err.Error())
		}
	}
}

func (gen *GenGo) genHead() {
	gen.code.WriteString(`//
// This file war generated by FastTars2go ` + VERSION + `
// Generated from ` + filepath.Base(gen.path) + `
// Tencent.

`)
}

func (gen *GenGo) genPackage() {
	gen.code.WriteString("package " + gen.p.Module + "\n\n")
}

func (gen *GenGo) genImport(module string) {
	for _, p := range gen.I {
		if strings.HasSuffix(p, "/"+module) {
			gen.code.WriteString(`"` + p + `"` + "\n")
			return
		}
	}
	gen.code.WriteString(`"` + module + `"` + "\n")
}

func (gen *GenGo) genStructPackage(st *StructInfo) {
	gen.code.WriteString("package " + gen.p.Module + "\n\n")
	gen.code.WriteString(`
import (
"fmt"
`)
	//"tars/protocol/codec"
	gen.code.WriteString("\"" + gen.tarsPath + "/protocol/codec\"\n")
	for k, _ := range st.DependModule {
		gen.genImport(k)
	}
	gen.code.WriteString(")" + "\n")

}

func (gen *GenGo) genIFPackage(itf *InterfaceInfo) {
	gen.code.WriteString("package " + gen.p.Module + "\n\n")
	gen.code.WriteString(`
import (
"fmt"
"unsafe"
`)
	gen.code.WriteString("\"" + gen.tarsPath + "/protocol/res/requestf\"\n")
	gen.code.WriteString("m \"" + gen.tarsPath + "/model\"\n")
	gen.code.WriteString("\"" + gen.tarsPath + "/protocol/codec\"\n")

	if *g_add_servant {
		gen.code.WriteString("\"" + gen.tarsPath + "\"\n")
	}
	for k, _ := range itf.DependModule {
		gen.genImport(k)
	}
	gen.code.WriteString(")" + "\n")

}

func (gen *GenGo) genType(ty *VarType) string {
	ret := ""
	switch ty.Type {
	case TK_T_BOOL:
		ret = "bool"
	case TK_T_INT:
		if ty.Unsigned {
			ret = "uint32"
		} else {
			ret = "int32"
		}
	case TK_T_SHORT:
		if ty.Unsigned {
			ret = "uint16"
		} else {
			ret = "int16"
		}
	case TK_T_BYTE:
		if ty.Unsigned {
			ret = "uint8"
		} else {
			ret = "int8"
		}
	case TK_T_LONG:
		if ty.Unsigned {
			ret = "uint64"
		} else {
			ret = "int64"
		}
	case TK_T_FLOAT:
		ret = "float32"
	case TK_T_DOUBLE:
		ret = "float64"
	case TK_T_STRING:
		ret = "string"
	case TK_T_VECTOR:
		ret = "[]" + gen.genType(ty.TypeK)
	case TK_T_MAP:
		ret = "map[" + gen.genType(ty.TypeK) + "]" + gen.genType(ty.TypeV)
	case TK_NAME:
		ret = strings.Replace(ty.TypeSt, "::", ".", -1)
		vec := strings.Split(ty.TypeSt, "::")
		for i, _ := range vec {
			if *g_add_servant == true {
				vec[i] = upperFirstLatter(vec[i])
			} else {
				if i == (len(vec) - 1) {
					vec[i] = upperFirstLatter(vec[i])
				}
			}
		}
		ret = strings.Join(vec, ".")
	default:
		gen.genErr("Unknow Type " + TokenMap[ty.Type])
	}
	return ret
}

func (gen *GenGo) genStructDefine(st *StructInfo) {
	c := &gen.code

	c.WriteString("type " + st.TName + " struct {\n")

	for _, v := range st.Mb {
		c.WriteString("\t" + v.Key + " " + gen.genType(v.Type) + "`json:\"" + v.KeyStr + "\"`\n")
	}
	c.WriteString("}\n")
}

func (gen *GenGo) genFunResetDefault(st *StructInfo) {
	c := &gen.code

	c.WriteString("func (st *" + st.TName + ") resetDefault() {\n")

	for _, v := range st.Mb {
		if v.Default == "" {
			continue
		}
		c.WriteString("st." + v.Key + " = " + v.Default + "\n")
	}
	c.WriteString("}\n")
}

func errString(hasRet bool) string {
	var ret_str string
	if hasRet {
		ret_str = "return ret, err"
	} else {
		ret_str = "return err"
	}
	return `if err != nil {
  ` + ret_str + `
  }`
}

func (gen *GenGo) genWriteSimpleList(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code
	tag := strconv.Itoa(int(mb.Tag))
	unsign := ""
	if mb.Type.TypeK.Unsigned {
		unsign = "u"
	}
	err_str := errString(hasRet)
	c.WriteString(`
err = _os.WriteHead(codec.SIMPLE_LIST, ` + tag + `)
` + err_str + `
err = _os.WriteHead(codec.BYTE, 0)
` + err_str + `
err = _os.Write_int32(int32(len(` + prefix + mb.Key + `)), 0)
` + err_str + `
err = _os.Write_slice_` + unsign + `int8(` + prefix + mb.Key + `)
` + err_str + `
`)
}

func (gen *GenGo) genWriteVector(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code

	// SIMPLE_LIST
	if mb.Type.TypeK.Type == TK_T_BYTE && !mb.Type.TypeK.Unsigned {
		gen.genWriteSimpleList(mb, prefix, hasRet)
		return
	}
	err_str := errString(hasRet)

	// LIST
	tag := strconv.Itoa(int(mb.Tag))
	c.WriteString(`
err = _os.WriteHead(codec.LIST, ` + tag + `)
` + err_str + `
err = _os.Write_int32(int32(len(` + prefix + mb.Key + `)), 0)
` + err_str + `
for _, v := range ` + prefix + mb.Key + ` {
`)
	// for _, v := range 里面再嵌套 for _, v := range也没问题，不会冲突，支持多维数组

	dummy := &StructMember{}
	dummy.Type = mb.Type.TypeK
	dummy.Key = "v"
	gen.genWriteVar(dummy, "", hasRet)

	c.WriteString("}\n")
}

func (gen *GenGo) genWriteStruct(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code
	tag := strconv.Itoa(int(mb.Tag))
	c.WriteString(`
err = ` + prefix + mb.Key + `.WriteBlock(_os, ` + tag + `)
` + errString(hasRet) + `
`)
}

func (gen *GenGo) genWriteMap(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code
	tag := strconv.Itoa(int(mb.Tag))
	vc := strconv.Itoa(gen.vc)
	gen.vc++
	err_str := errString(hasRet)
	c.WriteString(`
err = _os.WriteHead(codec.MAP, ` + tag + `)
` + err_str + `
err = _os.Write_int32(int32(len(` + prefix + mb.Key + `)), 0)
` + err_str + `
for k` + vc + `, v` + vc + ` := range ` + prefix + mb.Key + ` {
`)
	// for _, v := range 里面再嵌套 for _, v := range也没问题，不会冲突，支持多维数组

	dummy := &StructMember{}
	dummy.Type = mb.Type.TypeK
	dummy.Key = "k" + vc
	gen.genWriteVar(dummy, "", hasRet)

	dummy = &StructMember{}
	dummy.Type = mb.Type.TypeV
	dummy.Key = "v" + vc
	dummy.Tag = 1
	gen.genWriteVar(dummy, "", hasRet)

	c.WriteString("}\n")
}

func (gen *GenGo) genWriteVar(v *StructMember, prefix string, hasRet bool) {
	c := &gen.code

	switch v.Type.Type {
	case TK_T_VECTOR:
		gen.genWriteVector(v, prefix, hasRet)
	case TK_T_MAP:
		gen.genWriteMap(v, prefix, hasRet)
	case TK_NAME:
		if v.Type.CType == TK_ENUM {
			// TK_ENUM 枚举型处理
			tag := strconv.Itoa(int(v.Tag))
			c.WriteString(`
err = _os.Write_int32(int32(` + prefix + v.Key + `),` + tag + `)
` + errString(hasRet) + `
`)
		} else {
			gen.genWriteStruct(v, prefix, hasRet)
		}
	default:
		tag := strconv.Itoa(int(v.Tag))
		c.WriteString(`
err = _os.Write_` + gen.genType(v.Type) + `(` + prefix + v.Key + `, ` + tag + `)
` + errString(hasRet) + `
`)
	}
}

func (gen *GenGo) genFunWriteBlock(st *StructInfo) {
	c := &gen.code

	// WriteBlock函数头
	c.WriteString(`
func (st *` + st.TName + `) WriteBlock(_os *codec.Buffer, tag byte) error {
	var err error
	err = _os.WriteHead(codec.STRUCT_BEGIN, tag)
	if err != nil {
		return err
	}

  st.WriteTo(_os)

	err = _os.WriteHead(codec.STRUCT_END, 0)
	if err != nil {
		return err
	}
	return nil
}
`)
}

func (gen *GenGo) genFunWriteTo(st *StructInfo) {
	c := &gen.code

	c.WriteString(`
func (st *` + st.TName + `) WriteTo(_os *codec.Buffer) error {
	var err error
`)
	for _, v := range st.Mb {
		gen.genWriteVar(&v, "st.", false)
	}

	c.WriteString(`
	return nil
}
`)
}

func (gen *GenGo) genReadSimpleList(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code
	unsign := ""
	if mb.Type.TypeK.Unsigned {
		unsign = "u"
	}
	err_str := errString(hasRet)

	c.WriteString(`
err, _ = _is.SkipTo(codec.BYTE, 0, true)
` + err_str + `
err = _is.Read_int32(&length, 0, true)
` + err_str + `
err = _is.Read_slice_` + unsign + `int8(&` + prefix + mb.Key + `, length, true)
` + err_str + `
`)
}

func genForHead(vc string) string {
	i := `i` + vc
	e := `e` + vc
	return ` for ` + i + `,` + e + ` := int32(0),length;` + i + `<` + e + `;` + i + `++ `
}

func (gen *GenGo) genReadVector(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code
	err_str := errString(hasRet)

	// LIST
	tag := strconv.Itoa(int(mb.Tag))
	vc := strconv.Itoa(gen.vc)
	gen.vc++
	require := "false"
	if mb.Require {
		require = "true"
	}
	c.WriteString(`
err, have, ty = _is.SkipToNoCheck(` + tag + `,` + require + `)
` + err_str + `
`)
	if require == "false" {
		c.WriteString("if have {")
	}

	c.WriteString(`
if ty == codec.LIST {
	err = _is.Read_int32(&length, 0, true)
  ` + err_str + `
  ` + prefix + mb.Key + ` = make(` + gen.genType(mb.Type) + `, length, length)
  ` + genForHead(vc) + `{
`)

	dummy := &StructMember{}
	dummy.Type = mb.Type.TypeK
	dummy.Key = mb.Key + "[i" + vc + "]"
	gen.genReadVar(dummy, prefix, hasRet)

	c.WriteString(`}
} else if ty == codec.SIMPLE_LIST {
`)
	if mb.Type.TypeK.Type == TK_T_BYTE {
		gen.genReadSimpleList(mb, prefix, hasRet)
	} else {
		c.WriteString(`err = fmt.Errorf("type not support SIMPLE_LIST.")
    ` + err_str)
	}
	c.WriteString(`
} else {
  err = fmt.Errorf("require vector, but not.")
  ` + err_str + `
}
`)

	if require == "false" {
		c.WriteString("}\n")
	}
}

func (gen *GenGo) genReadStruct(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code
	tag := strconv.Itoa(int(mb.Tag))
	require := "false"
	if mb.Require {
		require = "true"
	}
	c.WriteString(`
err = ` + prefix + mb.Key + `.ReadBlock(_is, ` + tag + `, ` + require + `)
` + errString(hasRet) + `
`)
}

func (gen *GenGo) genReadMap(mb *StructMember, prefix string, hasRet bool) {
	c := &gen.code
	tag := strconv.Itoa(int(mb.Tag))
	err_str := errString(hasRet)
	vc := strconv.Itoa(gen.vc)
	gen.vc++
	require := "false"
	if mb.Require {
		require = "true"
	}
	c.WriteString(`
err, have = _is.SkipTo(codec.MAP, ` + tag + `, ` + require + `)
` + err_str + `
`)
	if require == "false" {
		c.WriteString("if have {")
	}
	c.WriteString(`
err = _is.Read_int32(&length, 0, true)
` + err_str + `
` + prefix + mb.Key + ` = make(` + gen.genType(mb.Type) + `)
` + genForHead(vc) + `{
	var k` + vc + ` ` + gen.genType(mb.Type.TypeK) + `
	var v` + vc + ` ` + gen.genType(mb.Type.TypeV) + `
`)

	dummy := &StructMember{}
	dummy.Type = mb.Type.TypeK
	dummy.Key = "k" + vc
	gen.genReadVar(dummy, "", hasRet)

	dummy = &StructMember{}
	dummy.Type = mb.Type.TypeV
	dummy.Key = "v" + vc
	dummy.Tag = 1
	gen.genReadVar(dummy, "", hasRet)

	c.WriteString(`
	` + prefix + mb.Key + `[k` + vc + `] = v` + vc + `
}
`)
	if require == "false" {
		c.WriteString("}\n")
	}
}

func (gen *GenGo) genReadVar(v *StructMember, prefix string, hasRet bool) {
	c := &gen.code

	switch v.Type.Type {
	case TK_T_VECTOR:
		gen.genReadVector(v, prefix, hasRet)
	case TK_T_MAP:
		gen.genReadMap(v, prefix, hasRet)
	case TK_NAME:
		if v.Type.CType == TK_ENUM {
			tag := strconv.Itoa(int(v.Tag))
			require := "false"
			if v.Require {
				require = "true"
			}
			c.WriteString(`
err = _is.Read_int32((*int32)(&` + prefix + v.Key + `),` + tag + `, ` + require + `)
` + errString(hasRet) + `
`)
		} else {
			gen.genReadStruct(v, prefix, hasRet)
		}
	default:
		tag := strconv.Itoa(int(v.Tag))
		require := "false"
		if v.Require {
			require = "true"
		}
		c.WriteString(`
err = _is.Read_` + gen.genType(v.Type) + `(&` + prefix + v.Key + `, ` + tag + `, ` + require + `)
` + errString(hasRet) + `
`)
	}
}

func (gen *GenGo) genFunReadFrom(st *StructInfo) {
	c := &gen.code

	c.WriteString(`
func (st *` + st.TName + `) ReadFrom(_is *codec.Reader) error {
	var err error
	var length int32
	var have bool
	var ty byte
	st.resetDefault()

`)

	for _, v := range st.Mb {
		gen.genReadVar(&v, "st.", false)
	}

	c.WriteString(`
	_ = length
	_ = have
	_ = ty
	return nil
}
`)
}

func (gen *GenGo) genFunReadBlock(st *StructInfo) {
	c := &gen.code

	c.WriteString(`
func (st *` + st.TName + `) ReadBlock(_is *codec.Reader, tag byte, require bool) error {
	var err error
	var have bool
	st.resetDefault()

	err, have = _is.SkipTo(codec.STRUCT_BEGIN, tag, require)
	if err != nil {
		return err
	}
  if !have {
    if require {
      return fmt.Errorf("require ` + st.TName + `, but not exist. tag %d", tag)    
    } else {
      return nil
    }
  }

  st.ReadFrom(_is)

	err = _is.SkipToStructEnd()
	if err != nil {
		return err
	}
	_ = have
	return nil
}
`)
}

func (gen *GenGo) genStruct(st *StructInfo) {
	gen.code.Reset()
	gen.vc = 0
	st.rename()

	gen.genHead()
	gen.genStructPackage(st)

	gen.genStructDefine(st)
	gen.genFunResetDefault(st)
	gen.genFunReadFrom(st)
	gen.genFunReadBlock(st)
	gen.genFunWriteTo(st)
	gen.genFunWriteBlock(st)

	gen.saveToSourceFile(st.TName + ".go")
}

func (gen *GenGo) makeEnumName(en *EnumInfo, mb *EnumMember) string {
	return upperFirstLatter(en.TName) + "_" + upperFirstLatter(mb.Key)
}

func (gen *GenGo) genEnum(en *EnumInfo) {
	if len(en.Mb) == 0 {
		return
	}

	gen.code.Reset()
	en.rename()
	gen.genHead()
	gen.genPackage()

	c := &gen.code

	c.WriteString("type " + en.TName + " int32\n")
	c.WriteString("const (\n")
	for _, v := range en.Mb {
		c.WriteString(gen.makeEnumName(en, &v) + ` = ` + strconv.Itoa(int(v.Value)) + "\n")
	}

	c.WriteString(")\n")

	gen.saveToSourceFile(en.TName + ".go")
}

func (gen *GenGo) genConst(cst []ConstInfo) {
	if len(cst) == 0 {
		return
	}

	gen.code.Reset()
	gen.genHead()
	gen.genPackage()

	c := &gen.code
	c.WriteString("const (\n")

	for _, v := range gen.p.Const {
		v.rename()
		c.WriteString(v.Key + " " + gen.genType(v.Type) + " = " + v.Value + "\n")
	}

	c.WriteString(")\n")

	realName := filepath.Base(gen.path)
	ss := strings.Split(realName, ".")
	if len(ss) >= 2 {
		realName = strings.Join(ss[:len(ss)-1], "")
	}
	gen.saveToSourceFile(realName + "_const.go")
}

func (gen *GenGo) genInclude(ps []*Parse) {
	for _, v := range ps {
		gen2 := &GenGo{path: v.Source, prefix: gen.prefix, tarsPath: g_tarsPath}
		gen2.p = v
		gen2.genAll()
	}
}

func (gen *GenGo) genAll() {
	if *g_add_servant == true {
		gen.p.rename()
	}
	gen.genInclude(gen.p.IncParse)

	for _, v := range gen.p.Enum {
		gen.genEnum(&v)
	}

	gen.genConst(gen.p.Const)

	for _, v := range gen.p.Struct {
		gen.genStruct(&v)
	}

	for _, v := range gen.p.Interface {
		gen.genInterface(&v)
	}
}

func (gen *GenGo) genInterface(itf *InterfaceInfo) {
	gen.code.Reset()
	itf.rename()

	gen.genHead()
	gen.genIFPackage(itf)

	gen.genIFProxy(itf)
	gen.genIFServer(itf)
	gen.genIFDispatch(itf)

	gen.saveToSourceFile(itf.TName + "_IF.go")
}

func (gen *GenGo) genIFProxy(itf *InterfaceInfo) {
	c := &gen.code
	c.WriteString("type " + itf.TName + " struct {" + "\n")
	c.WriteString("s m.Servant" + "\n")
	c.WriteString("}" + "\n")

	for _, v := range itf.Fun {
		gen.genIFProxyFun(itf.TName, &v)
	}

	c.WriteString(`
func (_obj *` + itf.TName + `) SetServant(s m.Servant) {
	_obj.s = s
}
`)
	c.WriteString(`
func (_obj *` + itf.TName + `) TarsSetTimeout(t int) {
	_obj.s.TarsSetTimeout(t)
}
`)
	c.WriteString(`
func (_obj *` + itf.TName + `)  byteToInt8(s []byte) []int8 {
    d := *(*[]int8)(unsafe.Pointer(&s))
    return d
}
func (_obj *` + itf.TName + `) int8ToByte(s []int8) []byte {
	d := *(*[]byte)(unsafe.Pointer(&s))
	return d
}
	`)

	if *g_add_servant {
		c.WriteString(`
func (_obj *` + itf.TName + `) AddServant(imp _imp` + itf.TName + `, obj string) {
  tars.AddServant(_obj, imp, obj)
}
`)
	}
}

func (gen *GenGo) genIFProxyFun(interfName string, fun *FunInfo) {
	c := &gen.code
	c.WriteString("func (_obj *" + interfName + ") " + fun.Name + "(")
	for _, v := range fun.Args {
		gen.genArgs(&v)
	}

	c.WriteString(" _opt ...map[string]string)")
	if fun.HasRet {
		c.WriteString("(ret " + gen.genType(fun.RetType) + ", err error){" + "\n")
	} else {
		c.WriteString("(err error)" + "{" + "\n")
	}

	c.WriteString(`
	var length int32
	var have bool
	var ty byte
  `)
	c.WriteString("_os := codec.NewBuffer()")
	var isOut bool
	for k, v := range fun.Args {
		if v.IsOut {
			isOut = true
		} else {
			dummy := &StructMember{}
			dummy.Type = v.Type
			dummy.Key = v.Name
			dummy.Tag = int32(k + 1)
			gen.genWriteVar(dummy, "", fun.HasRet)
		}
	}
	// empty args and below seperate
	c.WriteString("\n")
	err_str := errString(fun.HasRet)

	c.WriteString(`var _status map[string]string
var _context map[string]string
_resp := new(requestf.ResponsePacket)
err = _obj.s.Tars_invoke(0, "` + fun.NameStr + `", _os.ToBytes(), _status, _context, _resp)
` + err_str + `
`)

	if isOut || fun.HasRet {
		c.WriteString("_is := codec.NewReader(_obj.int8ToByte(_resp.SBuffer))")
	}
	if fun.HasRet {
		dummy := &StructMember{}
		dummy.Type = fun.RetType
		dummy.Key = "ret"
		dummy.Tag = 0
		dummy.Require = true
		gen.genReadVar(dummy, "", fun.HasRet)
	}

	for k, v := range fun.Args {
		if v.IsOut {
			dummy := &StructMember{}
			dummy.Type = v.Type
			dummy.Key = "(*" + v.Name + ")"
			dummy.Tag = int32(k + 1)
			dummy.Require = true
			gen.genReadVar(dummy, "", fun.HasRet)
		}
	}

	c.WriteString(`
  _ = length
  _ = have
  _ = ty
  `)

	if fun.HasRet {
		c.WriteString("return ret, nil" + "\n")
	} else {
		c.WriteString("return nil" + "\n")
	}

	c.WriteString("}" + "\n")
}

func (gen *GenGo) genArgs(arg *ArgInfo) {
	c := &gen.code
	c.WriteString(arg.Name + " ")
	if arg.IsOut || arg.Type.CType == TK_STRUCT {
		c.WriteString("*")
	}

	c.WriteString(gen.genType(arg.Type) + ",")
}

func (gen *GenGo) genIFServer(itf *InterfaceInfo) {
	c := &gen.code
	c.WriteString("type _imp" + itf.TName + " interface {" + "\n")
	for _, v := range itf.Fun {
		gen.genIFServerFun(&v)
	}
	c.WriteString("}" + "\n")
}

func (gen *GenGo) genIFServerFun(fun *FunInfo) {
	c := &gen.code
	c.WriteString(fun.Name + "(")
	for _, v := range fun.Args {
		gen.genArgs(&v)
	}
	c.WriteString(")(")

	if fun.HasRet {
		c.WriteString("ret " + gen.genType(fun.RetType) + ", ")
	}
	c.WriteString("err error)" + "\n")
}

func (gen *GenGo) genIFDispatch(itf *InterfaceInfo) {
	c := &gen.code
	c.WriteString("func(_obj *" + itf.TName + `) Dispatch(_val interface{}, req *requestf.RequestPacket, resp *requestf.ResponsePacket) (err error) {
	var length int32
	var have bool
	var ty byte
  `)

	var param bool
	for _, v := range itf.Fun {
		if len(v.Args) > 0 {
			param = true
			break
		}
	}

	if param {
		c.WriteString("_is := codec.NewReader(_obj.int8ToByte(req.SBuffer))")
	}
	c.WriteString(`
_os := codec.NewBuffer()
_imp := _val.(_imp` + itf.TName + `)
switch req.SFuncName {
`)

	for _, v := range itf.Fun {
		gen.genSwitchCase(&v)
	}

	c.WriteString(`
default:
	return fmt.Errorf("func mismatch")
}
var status map[string]string
*resp = requestf.ResponsePacket{
	IVersion:     1,
	CPacketType:  0,
	IRequestId:   req.IRequestId,
	IMessageType: 0,
	IRet:         0,
	SBuffer:      _obj.byteToInt8(_os.ToBytes()),
	Status:       status,
	SResultDesc:  "",
	Context:      req.Context,
}
_ = length
_ = have
_ = ty
return nil
}
`)
}

func (gen *GenGo) genSwitchCase(fun *FunInfo) {
	c := &gen.code
	c.WriteString(`case "` + fun.NameStr + `":` + "\n")

	for k, v := range fun.Args {
		c.WriteString("var " + v.Name + " " + gen.genType(v.Type))
		if !v.IsOut {
			dummy := &StructMember{}
			dummy.Type = v.Type
			dummy.Key = v.Name
			dummy.Tag = int32(k + 1)
			dummy.Require = true
			gen.genReadVar(dummy, "", false)
		} else {
			c.WriteString("\n")
		}
	}

	if fun.HasRet {
		c.WriteString("ret, err := _imp." + fun.Name + "(")
	} else {
		c.WriteString("err := _imp." + fun.Name + "(")
	}
	for _, v := range fun.Args {
		if v.IsOut || v.Type.CType == TK_STRUCT {
			c.WriteString("&" + v.Name + ",")
		} else {
			c.WriteString(v.Name + ",")
		}
	}
	c.WriteString(")")
	c.WriteString(`
if err != nil {
	return err
}
`)

	if fun.HasRet {
		dummy := &StructMember{}
		dummy.Type = fun.RetType
		dummy.Key = "ret"
		dummy.Tag = 0
		dummy.Require = true
		gen.genWriteVar(dummy, "", false)
	}
	for k, v := range fun.Args {
		if v.IsOut {
			dummy := &StructMember{}
			dummy.Type = v.Type
			dummy.Key = v.Name
			dummy.Tag = int32(k + 1)
			dummy.Require = true
			gen.genWriteVar(dummy, "", false)
		}
	}
}

func (gen *GenGo) Gen() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
		}
	}()

	gen.p = ParseFile(gen.path)
	gen.genAll()
}

func NewGenGo(path string, outdir string) *GenGo {
	if outdir != "" {
		b := []byte(outdir)
		last := b[len(b)-1:]
		if string(last) != "/" {
			outdir += "/"
		}
	}

	return &GenGo{path: path, prefix: outdir}
}
