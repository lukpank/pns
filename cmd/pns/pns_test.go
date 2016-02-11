// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import "testing"

func TestTagsURL(t *testing.T) {
	tests := []struct {
		path, expr, expected string
	}{
		{"/a", "b", "/a/b"},
		{"/a", "b c", "/a/b/c"},
		{"/a/b", "c", "/a/b/c"},
		{"/a/b", "c d", "/a/b/c/d"},

		{"/a", "/b", "/b"},
		{"/a/b", "/c", "/c/b"},
		{"/a", "/", "/"},
		{"/a/b", "/", "/-/b"},
		{"/a/b/c", "/", "/-/b/c"},

		{"/a/b", "-b", "/a"},
		{"/a/b/c", "-b", "/a/c"},
		{"/a/b/c", "-b -c", "/a"},
		{"/a/b/c/d", "-b -d", "/a/c"},

		{"/a", "-/a", "/"},
		{"/a/b", "-/a", "/-/b"},
		{"/a", "-/b", "/a"},
		{"/a/b", "-/c", "/a/b"},

		{"/a/b", "c -b", "/a/c"},
		{"/a/b", "-b c", "/a/c"},
		{"/a/b/c/d", "e f -b -d", "/a/c/e/f"},
		{"/a/b/c/d", "-b e -d f", "/a/c/e/f"},
	}
	for _, test := range tests {
		if s := tagsURL(test.path, test.expr); s != test.expected {
			t.Errorf("for (%q, %q) expected %q but got %q", test.path, test.expr, test.expected, s)
		}
	}
}