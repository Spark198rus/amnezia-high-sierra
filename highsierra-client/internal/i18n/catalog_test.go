package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// sourceDirs are the directories whose Go code shows text to people.
var sourceDirs = []string{"../../cmd/awg-hs", "../../internal/config", "../../internal/tunnel", "../../internal/control"}

type sources struct {
	calls   []string // arguments of i18n.T calls, resolved to their text
	strings []string // every constant string, with "a" + "b" joined
}

func readSources(t *testing.T) sources {
	t.Helper()
	var src sources
	for _, dir := range sourceDirs {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, pkg := range pkgs {
			consts := map[string]string{}
			for _, f := range pkg.Files {
				ast.Inspect(f, func(n ast.Node) bool {
					if spec, ok := n.(*ast.ValueSpec); ok {
						for i, name := range spec.Names {
							if i < len(spec.Values) {
								if s, ok := constString(spec.Values[i], nil); ok {
									consts[name.Name] = s
								}
							}
						}
					}
					return true
				})
			}
			for _, f := range pkg.Files {
				ast.Inspect(f, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.CallExpr:
						if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "T" {
							if x, ok := sel.X.(*ast.Ident); ok && x.Name == "i18n" && len(n.Args) == 1 {
								s, ok := constString(n.Args[0], consts)
								if !ok {
									t.Errorf("%s: i18n.T needs a constant text", fset.Position(n.Pos()))
								}
								src.calls = append(src.calls, s)
							}
						}
					case *ast.BinaryExpr, *ast.BasicLit:
						if s, ok := constString(n.(ast.Expr), nil); ok {
							src.strings = append(src.strings, s)
						}
					}
					return true
				})
			}
		}
	}
	return src
}

// constString evaluates a string literal, a concatenation of them, or (with
// consts) a named constant.
func constString(e ast.Expr, consts map[string]string) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		a, ok1 := constString(e.X, consts)
		b, ok2 := constString(e.Y, consts)
		return a + b, ok1 && ok2
	case *ast.ParenExpr:
		return constString(e.X, consts)
	case *ast.Ident:
		s, ok := consts[e.Name]
		return s, ok
	}
	return "", false
}

var goVerb = regexp.MustCompile(`%[-+# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)

func verbs(re *regexp.Regexp, s string) string {
	return strings.Join(re.FindAllString(s, -1), " ")
}

func TestCatalog(t *testing.T) {
	src := readSources(t)
	used := map[string]bool{}
	for _, s := range src.calls {
		used[s] = true
		tr, ok := ru[s]
		if !ok {
			t.Errorf("no Russian for %q", s)
			continue
		}
		if verbs(goVerb, s) != verbs(goVerb, tr) {
			t.Errorf("format verbs differ:\n  %q\n  %q", s, tr)
		}
		if strings.HasSuffix(s, "\n") != strings.HasSuffix(tr, "\n") {
			t.Errorf("trailing newline differs:\n  %q\n  %q", s, tr)
		}
	}
	for s := range ru {
		if !used[s] {
			t.Errorf("Russian text for %q, which the code no longer uses", s)
		}
	}
}

func TestPhrases(t *testing.T) {
	src := readSources(t)
	for _, p := range phrases {
		found := false
		for _, s := range src.strings {
			if strings.Contains(s, p.en) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("phrase %q no longer appears in the code", p.en)
		}
		if p.ru == "" || p.ru == p.en {
			t.Errorf("phrase %q has no Russian", p.en)
		}
	}
}

func TestMessage(t *testing.T) {
	for _, c := range []struct{ lang, in, want string }{
		{"ru", "line 2: PrivateKey: key must be 32 bytes of base64",
			"строка 2: PrivateKey: ключ должен быть 32 байтами в base64"},
		{"ru-RU", "resolving server address vpn.example.com: lookup failed",
			"не удалось найти адрес сервера vpn.example.com: lookup failed"},
		{"ru", "this is an Amnezia subscription key (Premium or free); only keys for self-hosted servers are supported",
			"это ключ подписки Amnezia (Premium или бесплатной); поддерживаются только ключи для VPN на своём сервере"},
		{"ru", "line 4: Junk: not a supported [Interface] setting",
			"строка 4: Junk: параметр не поддерживается в [Interface]"},
		{"en", "line 2: PrivateKey: key must be 32 bytes of base64",
			"line 2: PrivateKey: key must be 32 bytes of base64"},
		{"ru", "ifconfig utun3 up: exit status 1", "ifconfig utun3 up: exit status 1"},
	} {
		if got := Message(c.lang, c.in); got != c.want {
			t.Errorf("Message(%q, %q)\n got %q\nwant %q", c.lang, c.in, got, c.want)
		}
	}
}

func TestNormalizeAndDetect(t *testing.T) {
	for tag, want := range map[string]string{
		"ru": "ru", "ru-RU": "ru", "ru_RU.UTF-8": "ru", "RU": "ru",
		"en": "en", "en_US.UTF-8": "en", "uk-UA": "en", "": "en", "rus": "en",
	} {
		if got := Normalize(tag); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", tag, got, want)
		}
	}
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ru_RU.UTF-8")
	if got := Detect(); got != "ru" {
		t.Errorf("Detect with LANG=ru_RU.UTF-8 = %q", got)
	}
	t.Setenv("LC_ALL", "en_US.UTF-8")
	if got := Detect(); got != "en" {
		t.Errorf("Detect with LC_ALL=en_US.UTF-8 = %q", got)
	}
}

func TestT(t *testing.T) {
	defer func(l string) { Lang = l }(Lang)
	Lang = "ru"
	if got := T("Disconnected."); got != "Отключено." {
		t.Errorf("T = %q", got)
	}
	if got := T("not in the catalog"); got != "not in the catalog" {
		t.Errorf("T of an unknown text = %q", got)
	}
	Lang = "en"
	if got := T("Disconnected."); got != "Disconnected." {
		t.Errorf("T in English = %q", got)
	}
}

// The menu bar app's texts live in menubar/*.lproj/Localizable.strings.

var (
	nsLocalized = regexp.MustCompile(`NSLocalizedString\(@"((?:[^"\\]|\\.)*)"`)
	stringsLine = regexp.MustCompile(`^"((?:[^"\\]|\\.)*)"\s*=\s*"((?:[^"\\]|\\.)*)";$`)
	objcVerb    = regexp.MustCompile(`%(?:@|[-+# 0]*[0-9]*(?:\.[0-9]+)?(?:ll|l|h)?[dioux%fs])`)
)

func readStrings(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	inComment := false
	for n, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case inComment:
			inComment = !strings.Contains(line, "*/")
			continue
		case strings.HasPrefix(line, "/*"):
			inComment = !strings.Contains(line, "*/")
			continue
		case line == "":
			continue
		}
		m := stringsLine.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("%s:%d: can't read %q", path, n+1, line)
			continue
		}
		if _, dup := out[m[1]]; dup {
			t.Errorf("%s:%d: %q twice", path, n+1, m[1])
		}
		out[m[1]] = m[2]
	}
	return out
}

func TestMenubarStrings(t *testing.T) {
	sources, err := filepath.Glob("../../menubar/*.m")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no menubar sources: %v", err)
	}
	keys := map[string]bool{}
	for _, path := range sources {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range nsLocalized.FindAllStringSubmatch(string(b), -1) {
			keys[m[1]] = true
		}
	}

	en := readStrings(t, "../../menubar/en.lproj/Localizable.strings")
	ru := readStrings(t, "../../menubar/ru.lproj/Localizable.strings")
	for key := range keys {
		if en[key] != key {
			t.Errorf("en.lproj: %q should map to itself, got %q", key, en[key])
		}
		tr, ok := ru[key]
		if !ok {
			t.Errorf("ru.lproj: no Russian for %q", key)
			continue
		}
		if verbs(objcVerb, key) != verbs(objcVerb, tr) {
			t.Errorf("ru.lproj: format verbs differ:\n  %q\n  %q", key, tr)
		}
	}
	for _, table := range []map[string]string{en, ru} {
		for key := range table {
			if !keys[key] {
				t.Errorf("%q is translated but no longer used", key)
			}
		}
	}
}
