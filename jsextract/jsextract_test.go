package jsextract

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit: MaybeURL
// ---------------------------------------------------------------------------

func TestMaybeURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Should match
		{"/api/users", true},
		{"/api/v1/items?page=1", true},
		{"https://example.com", true},
		{"https://example.com/path", true},
		{"http://localhost:8080/api", true},
		{"//cdn.example.com/lib.js", true},
		{"api/v1/users", true},
		{"/dashboard", true},
		{"/login#redirect", true},

		// Should not match
		{"hello", false},
		{"", false},
		{"ab", false},
		{".foo", false},
		{".class-name", false},
		{"#id-selector", false},
		{":hover", false},
		{"text/html", false},
		{"application/json", false},
		{"image/png", false},
		{"data:image/png;base64,abc", false},
		{"javascript:void(0)", false},
		{"mailto:user@example.com", false},
		{"${variable}/path", false},
		{"2024/01/15", false},
		{"/*comment*/", false},
	}

	for _, tt := range tests {
		got := MaybeURL(tt.input)
		if got != tt.want {
			t.Errorf("MaybeURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Unit: each pattern
// ---------------------------------------------------------------------------

func TestFetchExtraction(t *testing.T) {
	src := `fetch("/api/users");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/users", "fetch")
}

func TestFetchSingleQuotes(t *testing.T) {
	src := `fetch('/api/items');`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/items", "fetch")
}

func TestImportExtraction(t *testing.T) {
	src := `import("/modules/lazy.js");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/modules/lazy.js", "import")
}

func TestXHRExtraction(t *testing.T) {
	src := `var xhr = new XMLHttpRequest(); xhr.open("GET", "/api/data");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/data", "xhr")
}

func TestJQueryGet(t *testing.T) {
	src := `$.get("/api/users");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/users", "jquery")
}

func TestJQueryPost(t *testing.T) {
	src := `jQuery.post("/api/submit");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/submit", "jquery")
}

func TestJQueryAjax(t *testing.T) {
	src := `$.ajax("/api/resource");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/resource", "jquery")
}

func TestJQueryGetJSON(t *testing.T) {
	src := `$.getJSON("/api/config");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/config", "jquery")
}

func TestLocationHref(t *testing.T) {
	src := `location.href = "/dashboard";`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/dashboard", "location")
}

func TestWindowLocationPathname(t *testing.T) {
	src := `window.pathname = "/page";`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/page", "location")
}

func TestLocationReplace(t *testing.T) {
	src := `location.replace("/new-page");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/new-page", "location")
}

func TestLocationAssign(t *testing.T) {
	src := `location.assign("/other-page");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/other-page", "location")
}

func TestWindowOpen(t *testing.T) {
	src := `window.open("/popup");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/popup", "window.open")
}

// ---------------------------------------------------------------------------
// Unit: string literal fallback
// ---------------------------------------------------------------------------

func TestStringLiteralFallback(t *testing.T) {
	src := `var config = { endpoint: "https://api.example.com/v2/data" };`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "https://api.example.com/v2/data", "string")
}

func TestStringLiteralRelativePath(t *testing.T) {
	src := `var url = "/api/v1/health";`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/v1/health", "string")
}

// ---------------------------------------------------------------------------
// Integration: multi-pattern
// ---------------------------------------------------------------------------

func TestMultiPatternExtraction(t *testing.T) {
	src := `
		fetch("/api/users");
		xhr.open("GET", "/api/data");
		$.get("/api/jquery");
		location.href = "/dashboard";
		window.open("/popup");
		var secret = "https://hidden.example.com/endpoint";
	`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]string{
		"/api/users":                          "fetch",
		"/api/data":                           "xhr",
		"/api/jquery":                         "jquery",
		"/dashboard":                          "location",
		"/popup":                              "window.open",
		"https://hidden.example.com/endpoint": "string",
	}

	if len(eps) != len(expected) {
		t.Errorf("got %d endpoints, want %d", len(eps), len(expected))
		for _, ep := range eps {
			t.Logf("  %s (%s)", ep.URL, ep.Type)
		}
	}

	for _, ep := range eps {
		wantType, ok := expected[ep.URL]
		if !ok {
			t.Errorf("unexpected endpoint: %q", ep.URL)
			continue
		}
		if ep.Type != wantType {
			t.Errorf("endpoint %q: got type %q, want %q", ep.URL, ep.Type, wantType)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: deduplication
// ---------------------------------------------------------------------------

func TestDeduplication(t *testing.T) {
	src := `
		fetch("/api/users");
		fetch("/api/users");
		var url = "/api/users";
	`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, ep := range eps {
		if ep.URL == "/api/users" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("got %d entries for /api/users, want 1 (dedup)", count)
	}
}

// ---------------------------------------------------------------------------
// Integration: structured patterns win over string fallback
// ---------------------------------------------------------------------------

func TestStructuredPriorityOverString(t *testing.T) {
	src := `fetch("/api/endpoint");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 {
		t.Fatalf("got %d endpoints, want 1", len(eps))
	}
	if eps[0].Type != "fetch" {
		t.Errorf("got type %q, want %q (structured should win)", eps[0].Type, "fetch")
	}
}

// ---------------------------------------------------------------------------
// Integration: katana compat
// ---------------------------------------------------------------------------

func TestExtractJsluiceEndpoints(t *testing.T) {
	data := `fetch("/api/users"); window.open("/popup");`
	eps := ExtractJsluiceEndpoints(data)
	if len(eps) < 2 {
		t.Fatalf("got %d endpoints, want at least 2", len(eps))
	}

	found := make(map[string]string)
	for _, ep := range eps {
		found[ep.Endpoint] = ep.Type
	}
	if found["/api/users"] != "fetch" {
		t.Errorf("missing or wrong type for /api/users: %q", found["/api/users"])
	}
	if found["/popup"] != "window.open" {
		t.Errorf("missing or wrong type for /popup: %q", found["/popup"])
	}
}

// ---------------------------------------------------------------------------
// String concatenation
// ---------------------------------------------------------------------------

func TestFetchStringConcat(t *testing.T) {
	src := `fetch("/api/users/" + userId);`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/users/EXPR", "fetch")
}

func TestFetchMultiConcat(t *testing.T) {
	src := `fetch("/api/" + version + "/users");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/EXPR/users", "fetch")
}

func TestFetchDeepConcat(t *testing.T) {
	src := `fetch("/api/" + a + "/" + b);`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/EXPR/EXPR", "fetch")
}

func TestXHRStringConcat(t *testing.T) {
	src := `xhr.open("GET", "/api/data/" + id);`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/data/EXPR", "xhr")
}

func TestFetchTemplateLiteral(t *testing.T) {
	src := "fetch(`/api/users/${userId}/posts`);"
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/users/EXPR/posts", "fetch")
}

func TestExtractorReuse(t *testing.T) {
	ext := NewExtractor()
	eps1, err := ext.Extract([]byte(`fetch("/api/a");`))
	if err != nil {
		t.Fatal(err)
	}
	eps2, err := ext.Extract([]byte(`fetch("/api/b");`))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps1, "/api/a", "fetch")
	assertEndpoint(t, eps2, "/api/b", "fetch")
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestTemplateLiteralResolved(t *testing.T) {
	src := "var url = `https://example.com/${userId}/profile`;"
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// Template literals with substitutions should resolve EXPR
	assertEndpoint(t, eps, "https://example.com/EXPR/profile", "string")
	for _, ep := range eps {
		if strings.Contains(ep.URL, "${") {
			t.Errorf("raw template substitution should not appear in URL: %q", ep.URL)
		}
	}
}

func TestEmptyStringSkipped(t *testing.T) {
	src := `var x = "";`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 0 {
		t.Errorf("empty string should not produce endpoints, got %d", len(eps))
	}
}

func TestQueryStringPreserved(t *testing.T) {
	src := `fetch("/api/search?q=test&page=1");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/api/search?q=test&page=1", "fetch")
}

func TestHashFragmentPreserved(t *testing.T) {
	src := `location.href = "/page#section";`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "/page#section", "location")
}

func TestFullURLInFetch(t *testing.T) {
	src := `fetch("https://api.example.com/v1/data");`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "https://api.example.com/v1/data", "fetch")
}

func TestProtocolRelativeURL(t *testing.T) {
	src := `var cdn = "//cdn.example.com/lib.js";`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint(t, eps, "//cdn.example.com/lib.js", "string")
}

func TestNoEndpointsInPlainCode(t *testing.T) {
	src := `var x = 42; var y = "hello"; console.log(x + y);`
	eps, err := ExtractEndpoints([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 0 {
		t.Errorf("plain code should not produce endpoints, got %d", len(eps))
		for _, ep := range eps {
			t.Logf("  %q (%s)", ep.URL, ep.Type)
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkExtractEndpoints(b *testing.B) {
	// ~2KB JS blob with various patterns
	src := []byte(`
		(function() {
			fetch("/api/users");
			fetch("/api/items?page=1");
			fetch("https://api.example.com/v2/data");

			var xhr = new XMLHttpRequest();
			xhr.open("GET", "/api/data");
			xhr.open("POST", "/api/submit");

			$.get("/api/jquery/get");
			$.post("/api/jquery/post");
			jQuery.ajax("/api/jquery/ajax");
			$.getJSON("/api/jquery/json");

			location.href = "/dashboard";
			location.replace("/new-page");
			location.assign("/other-page");
			window.pathname = "/page";

			window.open("/popup");
			window.open("https://example.com/external");

			var config = {
				apiUrl: "https://config.example.com/v1",
				wsUrl: "//ws.example.com/socket",
				path: "/static/assets/main.js",
			};

			import("/modules/lazy.js");

			var template = "Hello World";
			var num = 42;
			console.log("debug message");
		})();
	`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractEndpoints(src)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertEndpoint(t *testing.T, eps []Endpoint, url, typ string) {
	t.Helper()
	for _, ep := range eps {
		if ep.URL == url {
			if ep.Type != typ {
				t.Errorf("endpoint %q: got type %q, want %q", url, ep.Type, typ)
			}
			return
		}
	}
	t.Errorf("endpoint %q not found in results (got %d endpoints)", url, len(eps))
	for _, ep := range eps {
		t.Logf("  %q (%s)", ep.URL, ep.Type)
	}
}
