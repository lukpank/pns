// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestNotesTagURL(t *testing.T) {
	tests := []struct {
		path, tag, expected string
	}{
		{"/a", "/a", "/a"},
		{"/a", "/b", "/b"},
		{"/-", "/a", "/a"},
		{"/-/a", "/b", "/b/a"},
		{"/a/b", "/c", "/c/b"},

		{"/", "a", "/-/a"},
		{"/-", "a", "/-/a"},
		{"/a", "b", "/a/b"},
		{"/a/b", "c", "/a/b/c"},

		{"/a/b", "b", "/a/b"},
		{"/-/b", "b", "/-/b"},
		{"/a/b/c", "b", "/a/b/c"},
		{"/a/b/c", "c", "/a/b/c"},
	}
	const q = "?q=z"
	const q2 = "?q=z&start=100"
	for _, test := range tests {
		n := Notes{URL: test.path}
		if s := n.TagURL(test.tag); s != test.expected {
			t.Errorf("for (URL=%q, tag=%q) expected %q but got %q", n.URL, test.tag, test.expected, s)
		}
		n = Notes{URL: test.path + q}
		if s := n.TagURL(test.tag); s != test.expected+q {
			t.Errorf("for (URL=%q, tag=%q) expected %q but got %q", n.URL, test.tag, test.expected+q, s)
		}
		n = Notes{URL: test.path + q2}
		if s := n.TagURL(test.tag); s != test.expected+q {
			t.Errorf("for (URL=%q, tag=%q) expected %q but got %q", n.URL, test.tag, test.expected+q, s)
		}
	}
}

func TestTagsURL(t *testing.T) {
	tests := []struct {
		path, expr, expected string
	}{
		{"/a", "+b", "/a/b"},
		{"/a", "+ b c", "/a/b/c"},
		{"/a/b", "  +c", "/a/b/c"},
		{"/a/b", " + c d", "/a/b/c/d"},

		{"/a", "+/b", "/b"},
		{"/a/b", "+/c", "/c/b"},
		{"/a", "+ /", "/"},
		{"/a/b", " + /", "/-/b"},
		{"/a/b/c", "   + /", "/-/b/c"},

		{"/a/b", "-b", "/a"},
		{"/a/b/c", "-b", "/a/c"},
		{"/a/b/c", "-b -c", "/a"},
		{"/a/b/c/d", "-b -d", "/a/c"},

		{"/a", "-/a", "/"},
		{"/a/b", "-/a", "/-/b"},
		{"/a", "-/b", "/a"},
		{"/a/b", "-/c", "/a/b"},

		{"/a/b", "+c -b", "/a/c"},
		{"/a/b", "-b c", "/a/c"},
		{"/a/b/c/d", "  +  e f -b -d", "/a/c/e/f"},
		{"/a/b/c/d", "-b e -d f", "/a/c/e/f"},

		{"/a/b", "c d", "/-/c/d"},
		{"/a", "/d e f", "/d/e/f"},
		{"/a/b", "d /e f", "/e/d/f"},

		{"/a/b", "'c'", "/?q=c"},
		{"/a/b", "'c' d", "/-/d?q=c"},
		{"/a/b", `c 'd e' f "g"`, "/-/c/f?q=d+e+%22g%22"},
		{"/a/b", "'c' /d", "/d?q=c"},
		{"/a/b", `c 'd e' /f "g"`, "/f/c?q=d+e+%22g%22"},
		{"/a/b", "+'c'", "/a/b?q=c"},
		{"/a/b", "+ 'c' d", "/a/b/d?q=c"},
		{"/a/b", "+ c 'd e' f", "/a/b/c/f?q=d+e"},
	}
	for _, test := range tests {
		if s := tagsURL(test.path, test.expr, ""); s != test.expected {
			t.Errorf("for (%q, %q) expected %q but got %q", test.path, test.expr, test.expected, s)
		}
		if strings.IndexByte(test.expected, '?') >= 0 {
			if s := tagsURL(test.path, test.expr, `"z"`); s != test.expected {
				t.Errorf("for (%q, %q) expected %q but got %q", test.path, test.expr, test.expected, s)
			}
		} else {
			expected := test.expected + "?q=%22z%22"
			if s := tagsURL(test.path, test.expr, `"z"`); s != expected {
				t.Errorf("for (%q, %q) expected %q but got %q", test.path, test.expr, expected, s)
			}
		}

	}
}

func TestParseSearchExpr(t *testing.T) {
	tests := []struct {
		expr, expected string
	}{
		{`a b c`, `a.b.c#`},
		{`a b c `, `a.b.c#`},
		{`a bbb c `, `a.bbb.c#`},
		{`'a' b c`, `b.c#a`},
		{`a b 'c'`, `a.b#c`},
		{`a 'b' c`, `a.c#b`},
		{`a'b'c`, `a.c#b`},
		{`a 'b c' d`, `a.d#b c`},
		{`a 'b c`, `a#b c`},
		{`a 'b c `, `a#b c`},
		{`a '' b c`, `a.b.c#`},
		{`a ' ' b c`, `a.b.c#`},
		{`a b c ''`, `a.b.c#`},
		{`a b c ' '`, `a.b.c#`},
		{`a '' b '' c`, `a.b.c#`},
		{`"a" b c`, `b.c#"a"`},
		{`a "b c" d`, `a.d#"b c"`},
		{`a "b c`, `a#"b c"`},
		{`a "b c `, `a#"b c"`},
		{`a 'b' 'c' d`, `a.d#b c`},
		{`a 'b' c 'd'`, `a.c#b d`},
		{`a "b" "c" d`, `a.d#"b" "c"`},
		{`a "b" c "d"`, `a.c#"b" "d"`},
		{`a 'b' c "d e"`, `a.c#b "d e"`},
		{`a "b c" d "e f"`, `a.d#"b c" "e f"`},
		{`a "" b c`, `a.b.c#`},
		{`a " " b c`, `a.b.c#`},
		{`a "" b "" c`, `a.b.c#`},
		{`a "" b '' c`, `a.b.c#`},
		{`a "" b "  "`, `a.b#`},
	}
	for _, test := range tests {
		tokens, fts := parseSearchExpr(test.expr)
		s := fmt.Sprintf("%s#%s", strings.Join(tokens, "."), fts)
		if s != test.expected {
			t.Errorf("for %q expected %q but got %q", test.expr, test.expected, s)
		}
	}
}

func TestNotesActiveTagsURLs(t *testing.T) {
	tests := []struct {
		path     string
		expected []tagURL
	}{
		{"/", nil},
		{"/-", nil},
		{
			"/a", []tagURL{
				{"/a", "/"},
			},
		},
		{
			"/a/b", []tagURL{
				{"/a", "/-/b"},
				{"b", "/a"},
			},
		},
		{
			"/a/b/c", []tagURL{
				{"/a", "/-/b/c"},
				{"b", "/a/c"},
				{"c", "/a/b"},
			},
		},
		{
			"/-/b", []tagURL{
				{"b", "/"},
			},
		},
		{
			"/-/b/c", []tagURL{
				{"b", "/-/c"},
				{"c", "/-/b"},
			},
		},

		{
			"/?q=z", []tagURL{
				{"'z'", "/"},
			},
		},
		{
			"/-?q=z", []tagURL{
				{"'z'", "/"},
			},
		},
		{
			"/a?q=y+z", []tagURL{
				{"/a", "/?q=y+z"},
				{"'y z'", "/a"},
			},
		},
		{
			"/a/b?q=x&other=y+z&start=100", []tagURL{
				{"/a", "/-/b?q=x"},
				{"b", "/a?q=x"},
				{"'x'", "/a/b"},
			},
		},
		{
			"/a/b/c?q=%22z%22", []tagURL{
				{"/a", "/-/b/c?q=%22z%22"},
				{"b", "/a/c?q=%22z%22"},
				{"c", "/a/b?q=%22z%22"},
				{`'"z"'`, "/a/b/c"},
			},
		},
		{
			"/-/b?start=100&q=%22z%22&other=x+y", []tagURL{
				{"b", "/?q=%22z%22"},
				{`'"z"'`, "/-/b"},
			},
		},
		{
			"/-/b/c?other=w&start=100&q=%22x+y%22+z", []tagURL{
				{"b", "/-/c?q=%22x+y%22+z"},
				{"c", "/-/b?q=%22x+y%22+z"},
				{`'"x y" z'`, "/-/b/c"},
			},
		},
	}
	for _, test := range tests {
		n := Notes{URL: test.path}
		result := n.ActiveTagsURLs()
		if len(result) != len(test.expected) {
			t.Errorf("for (%q) expected %d items but got %d items", test.path, len(test.expected), len(result))
			continue
		}
		for i, tagURL := range result {
			if tagURL.Name != test.expected[i].Name || tagURL.URL != test.expected[i].URL {
				t.Errorf("for (%q)[%d].Name expected %q but got %q", test.path, i, test.expected[i], tagURL)
			}
		}
	}
}

func TestNotesIncStart(t *testing.T) {
	tests := []struct {
		path       string
		start, inc int
		expected   string
	}{
		{"/a", 0, 100, "/a?start=100"},
		{"/a", 10, 100, "/a?start=110"},
		{"/a?start=10", 10, 100, "/a?start=110"},
		{"/a", 0, -1, "/a"},
		{"/a", 30, -100, "/a"},
		{"/a?start=30", 30, -100, "/a"},

		{"/a?q=%22z%22", 0, 100, "/a?q=%22z%22&start=100"},
		{"/a?q=%22z%22", 10, 100, "/a?q=%22z%22&start=110"},
		{"/a?q=%22z%22&start=10", 10, 100, "/a?q=%22z%22&start=110"},
		{"/a?q=%22z%22", 0, -1, "/a?q=%22z%22"},
		{"/a?q=%22z%22", 30, -100, "/a?q=%22z%22"},
		{"/a?start=30&q=%22z%22", 30, -100, "/a?q=%22z%22"},
	}
	for _, test := range tests {
		paths := []string{test.path}
		if strings.IndexByte(test.path, '?') >= 0 {
			paths = append(paths, test.path+"&other=value", strings.Replace(test.path, "?", "?other=value&", 1))
		} else {
			paths = append(paths, test.path+"?other=value")
		}
		for _, path := range paths {
			n := Notes{URL: path, Start: test.start}
			if s := n.incStart(test.inc); s != test.expected {
				t.Errorf("for (%q, %d, %d) expected %q but got %q", path, test.start, test.inc, test.expected, s)
			}
		}
	}
}

func TestNotesSep(t *testing.T) {
	expected := "******\n"
	for _, s := range []string{
		"***\n**** \n*****\t\n* *****\n ******\n\t******\n",
		"***\n**** \n       \n** ****\n ******\n\t******\n*****",
		"*****\n**** \n***  \n*** ***\n ******\n\t******\n",
		"*****  \n**** \n***\n**** **\n ******\n\t******\n",
		"***** \t\n**** \n***\n***** *\n ******\n\t******\n",
		"***** \t\n****      \n*** ***\n ******\n\t******\n***",
	} {
		notes := []*Note{{Text: s}}
		if sep := string(notesSep(notes)); sep != expected {
			t.Errorf("for %q exected []byte(%q) but got []byte(%q)", s, expected, sep)
		}
	}
}

func TestAddedRemoved(t *testing.T) {
	tests := []struct {
		old, new, added, removed string
	}{
		{"a c b", "", "", "a b c"},
		{"", "a c b", "a b c", ""},
		{"a", "b", "b", "a"},
		{"a c b", "b c a", "", ""},
		{"a c b", "b c a y x", "x y", ""},
		{"a c b y x", "b c a", "", "x y"},
		{"a b c d e f", "d x b e y", "x y", "a c f"},
	}
	for _, test := range tests {
		added, removed := addedRemoved(strings.Fields(test.old), strings.Fields(test.new))
		if s := strings.Join(added, " "); s != test.added {
			t.Errorf("for (%q, %q) expected added=%q but got %q", test.old, test.new, test.added, s)
		}
		if s := strings.Join(removed, " "); s != test.removed {
			t.Errorf("for (%q, %q) expected added=%q but got %q", test.old, test.new, test.removed, s)
		}
	}
}

func TestSplitTokens(t *testing.T) {
	input := "  aąbc[i++] = test;\nąę"
	expected := []string{" ", " ", "aąbc", "[", "i", "+", "+", "]", " ", "=", " ", "test", ";", "\n", "ąę"}
	got := splitTokens(input)
	if len(got) != len(expected) {
		t.Errorf("expected %d tokens but got %d", len(expected), len(got))
	}
	n := len(expected)
	if len(got) < n {
		n = len(got)
	}
	for i := 0; i < n; i++ {
		if got[i] != expected[i] {
			t.Errorf("got[%d] = %q but expected %q", i, got[i], expected[i])
		}
	}
}

const testOldText = `Test1 delete:
"Delete" this text.

Test2 insert:

Test3 replace:
"Delete" what follows: Zażółć gęślą jaźń. Test.
"Replace": The brown dogs enter into a dense fog.

The end.
`

const testNewText = `Test1 delete:

Test2 insert:
"Insert" this text.

Test3 replace:
"Delete" what follows. Test. Inserted "Za".
"Replace": The brown "fox" enters into a dense fog.

The end.
`

const testExpectedDiff = `<div class="context">Test1 delete:
</div><div class="del">&#34;Delete&#34; this text.
</div><div class="context">
Test2 insert:
</div><div class="ins">&#34;Insert&#34; this text.
</div><div class="context">
Test3 replace:
</div><div class="del">&#34;Delete&#34; what follows<del>: Zażółć gęślą jaźń</del>. Test.
&#34;Replace&#34;: The brown <del>dogs</del> <del>enter</del> into a dense fog.
</div><div class="ins">&#34;Delete&#34; what follows. Test.<ins> Inserted &#34;Za&#34;.</ins>
&#34;Replace&#34;: The brown <ins>&#34;fox&#34;</ins> <ins>enters</ins> into a dense fog.
</div><div class="context">
The end.
</div>`

func TestHtmlDiff(t *testing.T) {
	var b bytes.Buffer
	err := htmlDiff(&b, "Test", "Test")
	if err != NoDifference {
		t.Error("expected NoDifference")
	}
	if b.Len() != 0 {
		t.Error("expected no data written")
		b.Reset()
	}
	checkHtmlDiff(t, `Test abc`, `Test def`, `<div class="del">Test <del>abc</del></div><div class="ins">Test <ins>def</ins></div>`)
	checkHtmlDiff(t, testOldText, testNewText, testExpectedDiff)
}

func checkHtmlDiff(t *testing.T, oldText, newText, expectedDiff string) {
	var b bytes.Buffer
	err := htmlDiff(&b, oldText, newText)
	if err != nil {
		t.Error("expected no error but got: ", err.Error())
		return
	}
	if b.String() != expectedDiff {
		t.Errorf(`expected "%s" but got "%s"`, expectedDiff, b.String())
	}
}
