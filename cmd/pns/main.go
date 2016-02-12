// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"database/sql"
	"errors"
	"flag"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bgentry/speakeasy"
	"github.com/golang-commonmark/markdown"
)

const cookieMaxAge = 3600

var (
	dbFileName = flag.String("f", "", "sqlite3 database file name")
	dbInit     = flag.Bool("init", false, "initialize the database file")
	dbAddUser  = flag.String("adduser", "", "add user with given login to the database file (asks for the password)")
	importFrom = flag.String("import", "", "import notes from given file")
	httpAddr   = flag.String("http", ":8080", "listen address")
)

func main() {
	flag.Parse()
	if *dbFileName == "" {
		log.Fatal("option -f is requiered")
	}
	db, err := OpenDB(*dbFileName)
	if err != nil {
		log.Fatal(err)
	}
	if *dbInit {
		if err = db.Init(); err != nil {
			log.Fatal("failed to initialize database: ", err)
		}
	}
	if *importFrom != "" {
		notes, err := parseFile(*importFrom)
		if err != nil {
			log.Fatal("failed to parse imported file: ", err)
		}
		if err := db.Import(notes); err != nil {
			log.Fatal("failed to import into database: ", err)
		}
	}
	if *dbAddUser != "" {
		pass, err := speakeasy.Ask("Password: ")
		if err != nil {
			log.Fatal("failed to add user: ", err)
		}
		repeat, err := speakeasy.Ask("Retype password: ")
		if err != nil {
			log.Fatal("failed to add user: ", err)
		}
		if repeat != pass {
			log.Fatal("failed to add user: passwords do not match")
		}
		if err = db.AddUser(*dbAddUser, []byte(pass)); err != nil {
			log.Fatal("failed to add user: ", err)
		}
	}
	if *dbInit || *importFrom != "" || *dbAddUser != "" {
		return
	}

	t, err := template.ParseFiles("templates/layout.html", "templates/edit.html", "templates/preview.html", "templates/login.html")
	if err != nil {
		log.Fatal(err)
	}
	s := &server{db, t, markdown.New(), NewSessions()}
	http.Handle("/", s.authenticate(s.ServeHTTP))
	http.HandleFunc("/_/edit/", s.authenticate(s.serveEdit))
	http.HandleFunc("/_/edit/preview/", s.authenticate(s.serveEditPreview))
	http.HandleFunc("/_/edit/submit/", s.authenticate(s.serveEditSubmit))
	http.Handle("/_/static/", http.StripPrefix("/_/static/", http.FileServer(http.Dir("./static/"))))
	http.HandleFunc("/_/login", s.serveLogin)
	http.HandleFunc("/_/logout/", s.serveLogout)
	log.Fatal(http.ListenAndServe(*httpAddr, logger{}))
}

type server struct {
	db *DB
	t  *template.Template
	md *markdown.Markdown
	s  *sessions
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	tags := strings.Split(path, "/")
	r.ParseForm()
	if tag := r.Form.Get("tag"); tag != "" {
		http.Redirect(w, r, tagsURL(path, tag), http.StatusMovedPermanently)
		return
	}
	var notes []*Note
	var err error
	var availableTags []string
	var isHTML bool
	if path == "/" || path == "/-" || path == "/-/" {
		notes, availableTags, err = s.db.TopicsAndTagsAsNotes()
		isHTML = true
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
	err = s.t.ExecuteTemplate(w, "layout.html", &Notes{path, notes, s.md, availableTags, isHTML})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveEdit(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path, "/_/edit/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	note, err := s.db.Note(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	topics, tags, err := s.db.TopicsAndTags()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tt := append(topics, tags...)
	noteEx := struct {
		*Note
		TopicsAndTagsComma string
		TopicsAndTagsSpace string
	}{note, strings.Join(tt, ", "), strings.Join(tt, " ")}
	err = s.t.ExecuteTemplate(w, "edit.html", noteEx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveEditPreview(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path, "/_/edit/preview/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	note, err := s.db.Note(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = s.t.ExecuteTemplate(w, "preview.html", &Notes{Notes: []*Note{note}, md: s.md})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveEditSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "please use POST", http.StatusMethodNotAllowed)
		return
	}
	id, err := idFromPath(r.URL.Path, "/_/edit/submit/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()
	if r.PostForm.Get("action") != "Preview" {
		http.Error(w, "for now only preview is supported", http.StatusBadRequest)
		_ = id // TODO use it in action Submit
		return
	}
	note := &Note{Text: r.PostForm.Get("text")}
	err = s.t.ExecuteTemplate(w, "preview.html", &Notes{Notes: []*Note{note}, md: s.md})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) authenticate(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("session_id"); err == nil && s.s.ValidSession(cookie.Value) {
			h(w, r)
		} else {
			s.loginPage(w, r, r.URL.Path, "")
		}
	}
}

func (s *server) serveLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "please use POST", http.StatusMethodNotAllowed)
		return
	}
	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	login := r.PostForm.Get("login")
	password := r.PostForm.Get("password")
	redirect := r.PostForm.Get("redirect")
	if err := s.db.AuthenticateUser(login, []byte(password)); err != nil {
		if err == ErrAuth {
			w.WriteHeader(http.StatusUnauthorized)
			s.loginPage(w, r, redirect, "Incorrect login or password.")
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	sid, err := s.s.NewSession(time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(cookieMaxAge * time.Second)
	http.SetCookie(w, &http.Cookie{Name: "session_id", Path: "/", Value: sid, MaxAge: cookieMaxAge, Expires: expires})
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *server) loginPage(w http.ResponseWriter, r *http.Request, path, msg string) {
	err := s.t.ExecuteTemplate(w, "login.html", struct{ Redirect, Message string }{path, msg})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		log.Println(err)
	} else {
		s.s.Remove(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "session_id", Path: "/", MaxAge: -1})
	path := strings.TrimPrefix(r.URL.Path, "/_/logout")
	if len(path) == len(r.URL.Path) || path == "" {
		path = "/"
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

var ErrPrefixNotFound = errors.New("prefix not found")

func idFromPath(path, prefix string) (int64, error) {
	idStr := strings.TrimPrefix(path, prefix)
	if len(idStr) == len(path) {
		return 0, ErrPrefixNotFound
	}
	return strconv.ParseInt(idStr, 10, 64)
}
