// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

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

func remoteAddr(r *http.Request) string {
	forward := r.Header.Get("X-Forwarded-For")
	if forward != "" {
		return fmt.Sprintf("%s (%s)", r.RemoteAddr, forward)
	}
	return r.RemoteAddr
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

type logger struct{}

func (logger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	rw := &responseWriter{w, 0, false}
	defer func() {
		log.Println(remoteAddr(r), r.Method, path, "-", rw.status, http.StatusText(rw.status), time.Since(t))
	}()
	http.DefaultServeMux.ServeHTTP(rw, r)
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

type Notes struct {
	URL           string
	Notes         []*Note
	md            *markdown.Markdown
	AvailableTags []string
}

func (n *Notes) TagURL(tag string) string {
	tags := strings.Split(n.URL[1:], "/")
	if strings.HasPrefix(tag, "/") {
		tags[0] = tag
		return strings.Join(tags, "/")
	} else {
		for _, t := range tags[1:] {
			if tag == t {
				return n.URL
			}
		}
		return n.URL + "/" + tag
	}
}

type topic struct {
	URL  string
	Name string
}

func (n *Notes) Topic() *topic {
	if n.URL == "/" || strings.HasPrefix(n.URL, "/-") {
		return nil
	}
	s := n.URL[1:]
	i := strings.Index(s, "/")
	if i < 0 {
		return &topic{"/", n.URL}
	} else {
		return &topic{"/-" + s[i:], n.URL[:i+1]}
	}

}

func (n *Notes) Tags() []string {
	return strings.Split(n.URL[1:], "/")[1:]
}

func (n *Notes) DelTagURL(tag string) string {
	tags := strings.Split(n.URL[1:], "/")
	for i, t := range tags[1:] {
		if tag == t {
			return "/" + strings.Join(append(tags[:i+1], tags[i+2:]...), "/")
		}
	}
	return n.URL
}

func (n *Notes) Render(text string) (template.HTML, error) {
	var b bytes.Buffer
	err := n.md.Render(&b, []byte(text))
	if err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func tagsFromNotes(notes []*Note) []string {
	m := make(map[string]struct{})
	for _, n := range notes {
		for _, s := range n.Topics {
			m[s] = struct{}{}
		}
		for _, s := range n.Tags {
			m[s] = struct{}{}
		}
	}
	var tags []string
	for s := range m {
		tags = append(tags, s)
	}
	sort.Strings(tags)
	return tags
}

type Note struct {
	Topics   []string
	Tags     []string
	Created  time.Time
	Modified time.Time
	ID       int64
	Text     string
	NoFooter bool
}
