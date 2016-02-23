// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import "testing"

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
	}
	for _, test := range tests {
		if s := tagsURL(test.path, test.expr); s != test.expected {
			t.Errorf("for (%q, %q) expected %q but got %q", test.path, test.expr, test.expected, s)
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
