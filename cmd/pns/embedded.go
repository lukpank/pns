// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

// +build embedded

package main

import (
	"html/template"
	"net/http"
	"path/filepath"
)

// newTemplates return templates parsed from static assets
func newTemplate(filenames ...string) (TemplateExecutor, error) {
	var t *template.Template
	for _, fn := range filenames {
		var err error
		name := filepath.Base(fn)
		s, err := FSString(false, "/"+fn)
		if err != nil {
			return nil, err
		}
		if t == nil {
			t, err = template.New(name).Parse(s)
		} else {
			_, err = t.New(name).Parse(s)
		}
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

func newDir(path string) http.FileSystem {
	return Dir(false, "/"+path)
}
