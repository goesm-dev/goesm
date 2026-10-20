package lower

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// A call of fmt.Print, Println or Printf, or of Fprint, Fprintln or Fprintf
// to os.Stdout or os.Stderr, writes the formatted string to the host's
// standard output or error with the fmt patch's printStd, instead of
// through os.Stdout: an *os.File, with internal/poll's FD and syscall's
// file table behind it. A program that prints only this way leaves out
// that code, about 16 KB gzipped. The string is what Sprint, Sprintln or
// Sprintf returns, lowered to concatenation where Sprintf's is
// (sprintf.go): Println(a, b) formats as Sprintf("%v %v\n", a, b) does.
//
// Only when the program never changes the variable (StdFixed): it assigns
// it, takes its address or calls its Close method nowhere. A file closed
// through an interface (io.Closer) is not seen; printStd then reports the
// host's error instead of os.ErrClosed.

// stdFiles are the variables of package os that printCall writes to.
var stdFiles = map[string]int{"Stdout": 1, "Stderr": 2}

// findStdWrites returns the names of stdFiles that a package other than
// os assigns, takes the address of or closes.
func findStdWrites(pkgs []*packages.Package) map[string]bool {
	written := map[string]bool{}
	for _, pkg := range pkgs {
		if pkg.PkgPath == "os" {
			continue
		}
		info := pkg.TypesInfo
		std := func(e ast.Expr) string {
			if sel, ok := ast.Unparen(e).(*ast.SelectorExpr); ok {
				if v, ok := info.Uses[sel.Sel].(*types.Var); ok && v.Pkg() != nil && v.Pkg().Path() == "os" && v.Parent() == v.Pkg().Scope() {
					if _, ok := stdFiles[v.Name()]; ok {
						return v.Name()
					}
				}
			}
			return ""
		}
		for _, f := range pkg.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					for _, l := range n.Lhs {
						if star, ok := ast.Unparen(l).(*ast.StarExpr); ok {
							l = star.X
						}
						if name := std(l); name != "" {
							written[name] = true
						}
					}
				case *ast.UnaryExpr:
					if name := std(n.X); n.Op.String() == "&" && name != "" {
						written[name] = true
					}
				case *ast.SelectorExpr:
					if name := std(n.X); name != "" && n.Sel.Name == "Close" {
						written[name] = true
					}
				}
				return true
			})
		}
	}
	return written
}

// StdFixed reports whether the program never changes the variable name of
// stdFiles (see printCall).
func (p *Program) StdFixed(name string) bool { return !p.stdWritten[name] }

// printCall lowers such a call, or returns false.
func (fe *funcEmitter) printCall(e *ast.CallExpr) (string, bool) {
	if !fe.inBody {
		return "", false
	}
	sel, ok := ast.Unparen(e.Fun).(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	fn, ok := fe.info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" || fn.Signature().Recv() != nil || fe.callBlocks(e) {
		return "", false
	}
	name, args := fn.Name(), e.Args
	fd := 1
	if strings.HasPrefix(name, "F") {
		if len(args) == 0 {
			return "", false
		}
		s, ok := ast.Unparen(args[0]).(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		v, ok := fe.info.Uses[s.Sel].(*types.Var)
		if !ok || v.Pkg() == nil || v.Pkg().Path() != "os" || v.Parent() != v.Pkg().Scope() {
			return "", false
		}
		if fd, ok = stdFiles[v.Name()]; !ok {
			return "", false
		}
		name, args = name[1:], args[1:]
		if !fe.pe.prog.StdFixed(v.Name()) {
			return "", false
		}
	} else if !fe.pe.prog.StdFixed("Stdout") {
		return "", false
	}
	if name != "Print" && name != "Println" && name != "Printf" {
		return "", false
	}
	scope := fn.Pkg().Scope()
	printStd, ok := scope.Lookup("printStd").(*types.Func)
	if !ok || !isLoweringHelper(printStd) {
		return "", false
	}
	sprint, ok := scope.Lookup("Sp" + name[1:]).(*types.Func)
	if !ok {
		return "", false
	}
	s := ""
	if !e.Ellipsis.IsValid() {
		if format, ok := printFormat(fe.info, name, args); ok {
			vals := args
			fallback := func(boxes string) string { return fe.nameOf(sprint) + "(" + boxes + ")" }
			if name == "Printf" {
				vals = args[1:]
				fallback = func(boxes string) string { return fe.nameOf(sprint) + "(" + fe.expr(args[0]) + ", " + boxes + ")" }
			}
			s, _ = fe.concatFormat(fn, format, vals, false, fallback)
		}
	}
	if s == "" {
		c := &ast.CallExpr{Fun: e.Fun, Args: args, Ellipsis: e.Ellipsis}
		s = fe.nameOf(sprint) + "(" + fe.args(c, sprint.Signature()) + ")"
	}
	return fe.mark(e) + fe.nameOf(printStd) + "(" + strconv.Itoa(fd) + ", " + s + ")", true
}

// printFormat returns the format that Sprintf formats the arguments of
// Sprint, Sprintln or Sprintf with as those do, if it is known.
func printFormat(info *types.Info, name string, args []ast.Expr) (string, bool) {
	switch name {
	case "Printf":
		if len(args) == 0 {
			return "", false
		}
		tv := info.Types[args[0]]
		if tv.Value == nil || tv.Value.Kind() != constant.String {
			return "", false
		}
		return constant.StringVal(tv.Value), true
	case "Println":
		if len(args) == 1 {
			if _, ok := info.TypeOf(args[0]).(*types.Tuple); ok {
				return "", false
			}
		}
		return strings.TrimPrefix(strings.Repeat(" %v", len(args)), " ") + "\n", true
	}
	// Print adds a space between operands when neither is a string, which
	// is known of operands that are not interfaces.
	var format strings.Builder
	prevString := false
	for i, a := range args {
		t := info.TypeOf(a)
		if _, ok := t.(*types.Tuple); ok || isIface(t) {
			return "", false
		}
		b, ok := t.Underlying().(*types.Basic)
		isString := ok && b.Info()&types.IsString != 0
		if i > 0 && !isString && !prevString {
			format.WriteByte(' ')
		}
		format.WriteString("%v")
		prevString = isString
	}
	return format.String(), true
}
