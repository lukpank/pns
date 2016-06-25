// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

// +build devel

package main

import (
	"html/template"
	"io"
	"net/http"
)

// newTemplates template executor which reloads templates before every
// use. Used for development of templates.
func newTemplate(funcMap template.FuncMap, filenames ...string) (TemplateExecutor, error) {
	return &tmpl{filenames, funcMap}, nil
}

type tmpl struct {
	filenames []string
	funcMap   template.FuncMap
}

func (tt *tmpl) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	t, err := template.New("html").Funcs(tt.funcMap).ParseFiles(tt.filenames...)
	if err != nil {
		return err
	}
	return t.ExecuteTemplate(wr, name, data)
}

func newDir(path string) http.Dir {
	return http.Dir(path)
}
