package grammars

import (
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewGoTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}

	if _, err := NewGoTokenSource([]byte("package main\n"), lang); err == nil {
		t.Fatal("expected error for language missing go token symbols")
	}
}

func TestNewGoTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}

	ts := NewGoTokenSourceOrEOF([]byte("package main\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestGoTokenSourceSkipToByteReseek(t *testing.T) {
	lang := GoLanguage()

	var b strings.Builder
	b.WriteString("package main\n\nfunc main() {\n")
	for i := 0; i < 900; i++ {
		b.WriteString("\tx := 1\n")
	}
	b.WriteString("\ttarget := 2\n")
	b.WriteString("}\n")
	src := []byte(b.String())

	targetOffset := strings.Index(b.String(), "target")
	if targetOffset < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewGoTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewGoTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(targetOffset))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < targetOffset {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, targetOffset)
	}
	if tok.Text != "target" {
		t.Fatalf("expected identifier token text %q, got %q", "target", tok.Text)
	}
}

func TestGoTokenSourceRuneLiteralColumnsCountUTF8Bytes(t *testing.T) {
	lang := GoLanguage()
	src := []byte("package p\nvar _ = []struct{ from, to rune }{{'Å', 'Å'}}\n")

	offset := strings.Index(string(src), "'Å'")
	if offset < 0 {
		t.Fatal("missing rune literal")
	}

	ts, err := NewGoTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewGoTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(offset))
	if tok.Text != "'Å'" {
		t.Fatalf("SkipToByte token = %q, want %q", tok.Text, "'Å'")
	}

	gotWidth := tok.EndPoint.Column - tok.StartPoint.Column
	wantWidth := uint32(len(tok.Text))
	if gotWidth != wantWidth {
		t.Fatalf("rune literal column width = %d, want %d", gotWidth, wantWidth)
	}
}

func TestGoTokenSourceSplitsInterpretedStringEscapes(t *testing.T) {
	lang := GoLanguage()
	src := []byte("package p\nvar _ = \"\\u13b0\\uab80\"\n")

	ts, err := NewGoTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewGoTokenSource failed: %v", err)
	}

	var saw []string
	for {
		tok := ts.Next()
		if tok.Symbol == 0 {
			break
		}
		if tok.StartByte < uint32(strings.Index(string(src), "\"\\u13b0\\uab80\"")) || tok.EndByte > uint32(len(src)) {
			continue
		}
		switch tok.Text {
		case "\"", "\\u13b0", "\\uab80":
			saw = append(saw, tok.Text)
		}
	}

	got := strings.Join(saw, ",")
	want := "\",\\u13b0,\\uab80,\""
	if got != want {
		t.Fatalf("interpreted string token split = %q, want %q", got, want)
	}
}
