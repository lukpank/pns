// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
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
	for _, test := range tests {
		n := Notes{URL: test.path}
		if s := n.TagURL(test.tag); s != test.expected {
			t.Errorf("for (URL=%q, tag=%q) expected %q but got %q", n.URL, test.tag, test.expected, s)
		}
		n = Notes{URL: test.path + q}
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
		if s := tagsURL(test.path, test.expr); s != test.expected {
			t.Errorf("for (%q, %q) expected %q but got %q", test.path, test.expr, test.expected, s)
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
			"/a/b?q=x&other=y+z", []tagURL{
				{"/a", "/-/b?q=x&other=y+z"},
				{"b", "/a?q=x&other=y+z"},
				{"'x'", "/a/b?other=y+z"},
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
			"/-/b?q=%22z%22&other=x+y", []tagURL{
				{"b", "/?q=%22z%22&other=x+y"},
				{`'"z"'`, "/-/b?other=x+y"},
			},
		},
		{
			"/-/b/c?other=w&q=%22x+y%22+z", []tagURL{
				{"b", "/-/c?other=w&q=%22x+y%22+z"},
				{"c", "/-/b?other=w&q=%22x+y%22+z"},
				{`'"x y" z'`, "/-/b/c?other=w"},
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
