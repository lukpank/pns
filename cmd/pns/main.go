// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-commonmark/markdown"
)

var httpAddr = flag.String("http", ":8080", "listen address")

func main() {
	flag.Parse()
	args := flag.Args()
	var notes []*Note
	var err error
	if len(args) > 0 {
		notes, err = parseFile(args[0])
	} else {
		notes, err = parse(os.Stdin)
	}
	if err != nil {
		log.Fatal(err)
	}
	db, err := OpenDB(":memory:")
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Init(); err != nil {
		log.Fatal(err)
	}
	if err := db.Import(notes); err != nil {
		log.Fatal(err)
	}
	t, err := template.ParseFiles("templates/layout.html")
	if err != nil {
		log.Fatal(err)
	}
	if len(args) > 1 {
		out, err := os.Create(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		err = t.ExecuteTemplate(out, "layout.html", &Notes{"/test", notes, markdown.New(), nil})
		if err != nil {
			log.Fatal(err)
		}
	} else {
		http.Handle("/", &server{db, t, markdown.New()})
		http.Handle("/_/static/", http.StripPrefix("/_/static/", http.FileServer(http.Dir("./static/"))))
		log.Fatal(http.ListenAndServe(*httpAddr, logger{}))
	}
}

type server struct {
	db *DB
	t  *template.Template
	md *markdown.Markdown
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	tags := strings.Split(path, "/")
	r.ParseForm()
	if tag := r.Form.Get("tag"); tag != "" {
		notes := Notes{URL: path}
		http.Redirect(w, r, notes.TagURL(tag), http.StatusMovedPermanently)
		return
	}
	var notes []*Note
	var err error
	var availableTags []string
	if path == "/" || path == "/-" {
		notes, availableTags, err = s.db.TopicsAndTags()
	} else {
		notes, err = s.db.Notes("/"+tags[1], tags[2:])
		availableTags = tagsFromNotes(notes)
	}
	if err != nil {
		if _, ok := err.(NoTagsError); ok {
			notes = nil
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if len(notes) == 0 {
		w.WriteHeader(http.StatusNotFound)
		notes = append(notes, &Note{
			Text:     "# No such notes",
			NoFooter: true,
		})
	}
	err = s.t.ExecuteTemplate(w, "layout.html", &Notes{path, notes, s.md, availableTags})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
