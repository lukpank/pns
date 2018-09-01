// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

// +build !embedded,!devel

package main

import (
	"html/template"
	"net/http"
)

// newTemplates return templates parsed from filesystem
func newTemplate(funcMap template.FuncMap, filenames ...string) (TemplateExecutor, error) {
	return template.New("html").Funcs(funcMap).ParseFiles(filenames...)
}

func newDir(path string) http.Dir {
	return http.Dir(path)
}
