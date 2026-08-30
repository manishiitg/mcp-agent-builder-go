package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A process started and never reaped is invisible until somebody takes a
// goroutine dump. PLAT-150: live-attach started its tmux control client with
// pty.StartWithSize and cleaned up with only ptmx.Close() + cmd.Process.Kill().
// Kill terminates a process but does NOT reap it — the child stays <defunct>
// until its parent calls Wait — and because it was an exec.CommandContext,
// os/exec's watchCtx goroutine also blocked forever on a send only Wait drains.
// Every attach leaked one PID and one goroutine, permanently, until restart.
//
// It survived review because the cleanup code LOOKS right ("on cancel, close
// the pty and kill the process"), the bug is a missing POSIX step rather than a
// confusing line, and it never produces an error or a failed request.
//
// A text heuristic does not catch this. Comparing per-file counts of ".Start()"
// against ".Wait()" reported this file clean, because the counts say nothing
// about WHICH object was started. So this walks the AST and tracks specific
// variables: only a variable assigned from exec.Command/exec.CommandContext is
// considered, and it must be reaped in the same function that starts it.
func TestEveryStartedExecCommandIsReaped(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve agent_go root: %v", err)
	}

	var findings []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable tree entries are not this test's concern
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "node_modules", ".git", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil // a file that does not parse is the compiler's problem
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}

		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			for _, f := range unreapedExecCommands(fn) {
				findings = append(findings, rel+":"+f)
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", root, walkErr)
	}

	sort.Strings(findings)
	if len(findings) > 0 {
		t.Fatalf("exec.Command/CommandContext started but never reaped (Wait) in the same function:\n  %s\n\n"+
			"Kill() terminates a process; it does NOT reap it. Without Wait the child stays <defunct> forever and, for "+
			"exec.CommandContext, os/exec's watchCtx goroutine leaks with it (PLAT-150).\n"+
			"Fix by calling cmd.Wait() on every exit path — a defer is usually right, since a function can also return "+
			"on its own while the context is still live.\n"+
			"Run/Output/CombinedOutput reap internally and are always fine.",
			strings.Join(findings, "\n  "))
	}
}

// unreapedExecCommands returns "funcName (var)" for every variable in fn that
// is assigned from exec.Command/exec.CommandContext, is then started, and is
// never waited on anywhere inside fn (including nested closures, which is where
// cleanup usually lives).
func unreapedExecCommands(fn *ast.FuncDecl) []string {
	execVars := map[string]bool{}
	started := map[string]bool{}
	waited := map[string]bool{}

	recordAssign := func(lhs []ast.Expr, rhs []ast.Expr) {
		if len(lhs) == 0 || len(rhs) != 1 {
			return
		}
		call, ok := rhs[0].(*ast.CallExpr)
		if !ok {
			return
		}
		if !isSelectorCall(call, "exec", "Command", "CommandContext") {
			return
		}
		if ident, ok := lhs[0].(*ast.Ident); ok && ident.Name != "_" {
			execVars[ident.Name] = true
		}
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			recordAssign(node.Lhs, node.Rhs)
		case *ast.CallExpr:
			// cmd.Start() / cmd.Wait()
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if recv, ok := sel.X.(*ast.Ident); ok {
					switch sel.Sel.Name {
					case "Start":
						started[recv.Name] = true
					case "Wait":
						waited[recv.Name] = true
					}
				}
			}
			// pty.Start(cmd) / pty.StartWithSize(cmd, ...) start their argument
			// rather than being called on it, so the receiver-based check above
			// cannot see them — this is exactly the PLAT-150 shape.
			if isSelectorCall(node, "pty", "Start", "StartWithSize") && len(node.Args) > 0 {
				if ident, ok := node.Args[0].(*ast.Ident); ok {
					started[ident.Name] = true
				}
			}
		}
		return true
	})

	var out []string
	for name := range started {
		if execVars[name] && !waited[name] {
			out = append(out, fn.Name.Name+" ("+name+")")
		}
	}
	sort.Strings(out)
	return out
}

// isSelectorCall reports whether call is pkg.Name(...) for any of names.
func isSelectorCall(call *ast.CallExpr, pkg string, names ...string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != pkg {
		return false
	}
	for _, name := range names {
		if sel.Sel.Name == name {
			return true
		}
	}
	return false
}
