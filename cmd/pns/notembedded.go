// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

// +build !embedded

package main

import (
	"html/template"
	"net/http"
)

// newTemplates return templates parsed from filesystem
func newTemplate(filenames ...string) (*template.Template, error) {
	return template.ParseFiles(filenames...)
}

func newDir(path string) http.Dir {
	return http.Dir(path)
}
