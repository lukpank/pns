// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bgentry/speakeasy"
	"github.com/golang-commonmark/markdown"
)

const (
	sessionDuration   = 3600 // session duration in seconds
	sessionCookieName = "pns_sid"
)

var (
	dbFileName = flag.String("f", "", "sqlite3 database file name")
	dbInit     = flag.Bool("init", false, "initialize the database file")
	dbAddUser  = flag.String("adduser", "", "add user with given login to the database file (asks for the password)")
	importFrom = flag.String("import", "", "import notes from given file")
	exportPath = flag.String("export", "", `export path, use "/" for all notes`)
	outFile    = flag.String("o", "", "output path, use with -export")
	httpAddr   = flag.String("http", "", "HTTP listen address")
	httpsAddr  = flag.String("https", "", "HTTPS listen address")
	certFile   = flag.String("https_cert", "", "HTTPS server certificate file")
	keyFile    = flag.String("https_key", "", "HTTPS server private key file")
	hostname   = flag.String("host", "", "reject requests with host other than this")
	version    = flag.Bool("v", false, "show program version")

	Version = "pns-0.1-(REV?)"
)

func main() {
	flag.Parse()
	if *version {
		fmt.Println(Version)
		return
	}
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
	if *exportPath != "" {
		var w io.Writer
		if *outFile != "" {
			f, err := os.Create(*outFile)
			if err != nil {
				log.Fatal("failed to export: ", err)
			}
			defer f.Close()
			w = f
		} else {
			w = os.Stdout
		}
		var notes []*Note
		if (*exportPath)[0] != '/' {
			log.Fatal("failed to export: export path must start with '/'")
		} else if *exportPath == "/" {
			notes, err = db.AllNotes()
		} else {
			tags := strings.Split(*exportPath, "/")
			notes, err = db.Notes("/"+tags[1], tags[2:], "", 0, false)
		}
		if err == nil {
			err = export(w, notes)
		}
		if err != nil {
			log.Fatal("failed to export: ", err)
		}
	}
	if *dbInit || *importFrom != "" || *dbAddUser != "" || *exportPath != "" {
		return
	}
	if *httpAddr == "" && *httpsAddr == "" {
		log.Fatal("please specify some action (for example -http or -https)")
	}
	if *httpAddr != "" && *httpsAddr != "" {
		log.Fatal("please specify either -http or -https listen address but not both")
	}
	if *httpsAddr != "" && (*certFile == "" || *keyFile == "") {
		log.Fatal("-https option requires -https_cert and -https_key options")
	}

	t, err := newTemplate("templates/layout.html", "templates/edit.html", "templates/preview.html", "templates/login.html", "templates/diff.html")
	if err != nil {
		log.Fatal(err)
	}
	s := &server{db, t, markdown.New(), NewSessions(), *httpsAddr != ""}
	http.Handle("/", s.authenticate(s.ServeHTTP))
	http.HandleFunc("/_/edit/", s.authenticate(s.serveEdit))
	http.HandleFunc("/_/edit/preview/", s.authenticate(s.serveEditPreview))
	http.HandleFunc("/_/edit/submit/", s.authenticate(s.serveEditSubmit))
	http.HandleFunc("/_/add", s.authenticate(s.serveAdd))
	http.HandleFunc("/_/add/submit", s.authenticate(s.serveAddSubmit))
	http.HandleFunc("/_/copy/", s.authenticate(s.serveCopy))
	http.Handle("/_/static/", http.StripPrefix("/_/static/", http.FileServer(newDir("static/"))))
	http.HandleFunc("/_/login", s.serveLogin)
	http.HandleFunc("/_/logout/", s.serveLogout)
	http.HandleFunc("/_/", s.authenticate(s.notFound))
	var h http.Handler = http.DefaultServeMux
	if *hostname != "" {
		h = newHostChecker(*hostname, h)
	}
	h = &logger{h}
	if *httpsAddr != "" {
		log.Fatal(http.ListenAndServeTLS(*httpsAddr, *certFile, *keyFile, h))
	} else {
		log.Fatal(http.ListenAndServe(*httpAddr, h))
	}
}

type server struct {
	db     *DB
	t      TemplateExecutor
	md     *markdown.Markdown
	s      *sessions
	secure bool
}

type TemplateExecutor interface {
	ExecuteTemplate(wr io.Writer, name string, data interface{}) error
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	tags := strings.Split(path, "/")
	r.ParseForm()
	if tag := r.Form.Get("tag"); tag != "" {
		http.Redirect(w, r, tagsURL(path, tag, r.Form.Get("q")), http.StatusMovedPermanently)
		return
	}
	var (
		notes         []*Note
		err           error
		allTags       []string
		activeTags    []string
		availableTags []string
		isHTML        = false
		count         = 0
		start         = 0
		more          = false
	)
	if path == "/" || path == "/-" || path == "/-/" {
		if q := r.Form.Get("q"); q != "" {
			start, err = strconv.Atoi(r.Form.Get("start"))
			if err != nil {
				start = 0
			}
			notes, err = s.db.FTS(q, start)
			if len(notes) > queryLimit {
				more = true
				notes = notes[:queryLimit]
			}
			count = len(notes)
		} else {
			notes, availableTags, err = s.db.TopicsAndTagsAsNotes()
			allTags = availableTags
			isHTML = true
		}
		activeTags = make([]string, 0)
	} else {
		start, err = strconv.Atoi(r.Form.Get("start"))
		if err != nil {
			start = 0
		}
		notes, err = s.db.Notes("/"+tags[1], tags[2:], r.Form.Get("q"), start, true)
		if len(notes) > queryLimit {
			more = true
			notes = notes[:queryLimit]
		}
		count = len(notes)
		availableTags = tagsFromNotes(notes)
		if availableTags == nil {
			availableTags = make([]string, 0)
		}
		activeTags = append([]string{"/" + tags[1]}, tags[2:]...)
	}
	if allTags == nil {
		var topics, tags []string
		topics, tags, err = s.db.TopicsAndTags()
		allTags = append(topics, tags...)
	}
	if err != nil {
		if _, ok := err.(NoTagsError); ok {
			notes = nil
		} else {
			s.internalError(w, err)
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
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	err = s.t.ExecuteTemplate(w, "layout.html", &Notes{path, notes, s.md, allTags, activeTags, availableTags, isHTML, nil, count, start, more})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveEdit(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path, "/_/edit/")
	if err != nil {
		s.notFound(w, r)
		return
	}
	note, err := s.db.Note(id)
	if err == sql.ErrNoRows {
		s.notFound(w, r)
		return
	} else if err != nil {
		s.internalError(w, err)
		return
	}
	ntt := append(note.Topics, note.Tags...)
	s.editPage(w, r, note, strings.Join(ntt, " "), note.sha1sum(), false)
}

func (s *server) editPage(w http.ResponseWriter, r *http.Request, note *Note, noteTopicsAndTags, sha1sum string, editConflict bool) {
	topics, tags, err := s.db.TopicsAndTags()
	if err != nil {
		s.internalError(w, err)
		return
	}
	tt := append(topics, tags...)
	noteEx := struct {
		*Note
		TopicsAndTagsComma string
		NoteTopicsAndTags  string
		Edit               bool
		EditConflict       bool
		Copy               bool
		SHA1Sum            string
		Referer            string
	}{note, strings.Join(tt, ", "), noteTopicsAndTags, true, editConflict, false, sha1sum, r.Header.Get("Referer")}
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
		s.internalError(w, err)
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
		s.error(w, "Method not allowed", "Please use POST.", http.StatusMethodNotAllowed)
		return
	}
	id, err := idFromPath(r.URL.Path, "/_/edit/submit/")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()
	text := r.PostForm.Get("text")
	tags := r.PostForm.Get("tag")
	switch r.PostForm.Get("action") {
	case "Preview":
		s.previewNote(w, r, id, text, strings.Fields(tags))
	case "Diff":
		s.diff(w, r, id, text, strings.Fields(tags), false)
	case "DiffConflict":
		s.diff(w, r, id, text, strings.Fields(tags), true)
	case "Submit":
		s.updateNote(w, r, id, text, tags, r.PostForm.Get("sha1sum"))
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
	}
}

// previewNote serves preview of a note intended for the iframe
// element of the edit page.
func (s *server) previewNote(w http.ResponseWriter, r *http.Request, id int64, text string, tags []string) {
	var dbTags []string
	if id >= 0 {
		note, err := s.db.Note(id)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dbTags = append(note.Topics, note.Tags...)
	}
	messages, err := s.preSubmitWarnings(tags, dbTags, id >= 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	note := &Note{Text: text}
	err = s.t.ExecuteTemplate(w, "preview.html", &Notes{Notes: []*Note{note}, md: s.md, Messages: messages})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) diff(w http.ResponseWriter, r *http.Request, id int64, text string, tags []string, conflict bool) {
	note, err := s.db.Note(id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		s.internalError(w, err)
		return
	}
	messages, err := s.preSubmitWarnings(tags, append(note.Topics, note.Tags...), true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	if conflict {
		messages = append([]string{"Conflicting edits detected. Please join the changes and click submit again when done."}, messages...)
	}
	var b bytes.Buffer
	err = htmlDiff(&b, strings.Replace(note.Text, "\r\n", "\n", -1), strings.Replace(text, "\r\n", "\n", -1))
	if err == NoDifference {
		messages = append(messages, "No differences found.")
	}
	data := struct {
		Diff     template.HTML
		Messages []string
	}{template.HTML(b.String()), messages}
	err = s.t.ExecuteTemplate(w, "diff.html", &data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) preSubmitWarnings(tags, dbTags []string, edit bool) ([]string, error) {
	var messages []string
	hasTopic := false
	for _, s := range tags {
		if s[0] == '/' {
			hasTopic = true
			break
		}
	}
	if !hasTopic {
		messages = append(messages, "Please specify at least one topic.")
	}
	if len(tags) > 0 {
		newTags, err := s.db.NewTags(tags)
		if err != nil {
			return nil, err
		}
		if len(newTags) > 0 {
			newStr := strings.Join(newTags, `" and "`)
			messages = append(messages, fmt.Sprintf(`Note that the following tags/topics are new: "%s".`, newStr))
		}
	}
	if edit {
		added, removed := addedRemoved(dbTags, tags)
		if len(added) > 0 {
			s := strings.Join(added, `" and "`)
			messages = append(messages, fmt.Sprintf(`You are adding the following tags/topics: "%s".`, s))
		}
		if len(removed) > 0 {
			s := strings.Join(removed, `" and "`)
			messages = append(messages, fmt.Sprintf(`You are removing the following tags/topics: "%s".`, s))
		}
	}
	return messages, nil
}

func addedRemoved(old, new []string) ([]string, []string) {
	old = append([]string{}, old...)
	new = append([]string{}, new...)
	sort.Strings(old)
	sort.Strings(new)
	var added, removed []string
	i := 0
	j := 0
	for i < len(old) && j < len(new) {
		switch {
		case old[i] == new[j]:
			i++
			j++
		case old[i] < new[j]:
			removed = append(removed, old[i])
			i++
		default:
			added = append(added, new[j])
			j++
		}
	}
	for i < len(old) {
		removed = append(removed, old[i])
		i++
	}
	for j < len(new) {
		added = append(added, new[j])
		j++
	}
	return added, removed
}

func (s *server) updateNote(w http.ResponseWriter, r *http.Request, id int64, text, topicsAndTags, sha1sum string) {
	topics, tags := topicsAndTagsFromEditField(topicsAndTags)
	err := s.db.updateNote(id, text, append(topics, tags...), sha1sum)
	if err == ErrNoTags {
		s.error(w, "Error", "Please specify at least one topic or tag.", http.StatusBadRequest)
		return
	} else if e, ok := err.(*EditConflictError); ok {
		s.editPage(w, r, &Note{ID: id, Text: text}, topicsAndTags, e.SHA1Sum, true)
		return
	} else if err != nil {
		s.internalError(w, err)
		return
	}
	path := editRedirectionPath(topics, tags, id)
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func (s *server) serveAdd(w http.ResponseWriter, r *http.Request) {
	topics, tags, err := s.db.TopicsAndTags()
	if err != nil {
		s.internalError(w, err)
		return
	}
	tt := append(topics, tags...)
	noteEx := struct {
		Text               string
		TopicsAndTagsComma string
		NoteTopicsAndTags  string
		Edit               bool
		EditConflict       bool
		Copy               bool
		Referer            string
	}{"", strings.Join(tt, ", "), "", false, false, false, r.Header.Get("Referer")}
	err = s.t.ExecuteTemplate(w, "edit.html", noteEx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveCopy(w http.ResponseWriter, r *http.Request) {
	id, err := idFromPath(r.URL.Path, "/_/copy/")
	if err != nil {
		s.notFound(w, r)
		return
	}
	note, err := s.db.Note(id)
	if err == sql.ErrNoRows {
		s.notFound(w, r)
		return
	} else if err != nil {
		s.internalError(w, err)
		return
	}
	ntt := append(note.Topics, note.Tags...)

	topics, tags, err := s.db.TopicsAndTags()
	if err != nil {
		s.internalError(w, err)
		return
	}
	tt := append(topics, tags...)
	noteEx := struct {
		*Note
		TopicsAndTagsComma string
		NoteTopicsAndTags  string
		Edit               bool
		EditConflict       bool
		Copy               bool
		Referer            string
	}{note, strings.Join(tt, ", "), strings.Join(ntt, " "), false, false, true, r.Header.Get("Referer")}
	err = s.t.ExecuteTemplate(w, "edit.html", noteEx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveAddSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "Method not allowed", "Please use POST.", http.StatusMethodNotAllowed)
		return
	}
	r.ParseForm()
	text := r.PostForm.Get("text")
	tags := r.PostForm.Get("tag")
	switch r.PostForm.Get("action") {
	case "Preview":
		s.previewNote(w, r, -1, text, strings.Fields(tags))
	case "Submit":
		s.addNote(w, r, text, tags)
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
	}
}

func (s *server) addNote(w http.ResponseWriter, r *http.Request, text, topicsAndTags string) {
	topics, tags := topicsAndTagsFromEditField(topicsAndTags)
	id, err := s.db.addNote(text, append(topics, tags...))
	if err == ErrNoTags {
		s.error(w, "Error", "Please specify at least one topic or tag.", http.StatusBadRequest)
		return
	} else if err != nil {
		s.internalError(w, err)
		return
	}
	path := editRedirectionPath(topics, tags, id)
	http.Redirect(w, r, path, http.StatusSeeOther)
}

var errorTemplate = template.Must(template.New("tags").Parse("<h1>{{.Title}}</h1><p>{{.Text}}</p>"))

func (s *server) error(w http.ResponseWriter, title, text string, code int) {
	w.WriteHeader(code)
	var b bytes.Buffer
	errorTemplate.Execute(&b, &struct{ Title, Text string }{title, text})
	n := &Note{Text: b.String(), NoFooter: true}
	err := s.t.ExecuteTemplate(w, "layout.html", &Notes{"/", []*Note{n}, s.md, []string{}, []string{}, []string{}, true, nil, 0, 0, false})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	s.error(w, "Page not found", "", http.StatusNotFound)
}

func (s *server) internalError(w http.ResponseWriter, err error) {
	s.error(w, "Internal server error", err.Error(), http.StatusInternalServerError)
}

func editRedirectionPath(topics, tags []string, id int64) string {
	var topic string
	if len(topics) > 0 {
		topic = topics[0]
	} else if len(tags) > 0 {
		topic = "/-"
	} else {
		topic = "/"
	}
	if len(tags) > 0 {
		return fmt.Sprintf("%s/%s#%d", topic, strings.Join(tags, "/"), id)
	} else if topic != "/" {
		return fmt.Sprintf("%s#%d", topic, id)
	} else {
		return "/"
	}
}

func (s *server) authenticate(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			var extend bool
			if extend, err = s.s.CheckSession(cookie.Value, sessionDuration*time.Second); err == nil {
				if extend {
					s.setSessionCookie(w, cookie.Value, 2*sessionDuration)
				}
				h(w, r)
				return
			}
		}
		if err != nil && err != ErrAuth && err != http.ErrNoCookie {
			s.internalError(w, err)
			return
		}
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		s.loginPage(w, r, path, "")
	}
}

func (s *server) serveLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.error(w, "Method not allowed", "Please use POST.", http.StatusMethodNotAllowed)
		return
	}
	err := r.ParseForm()
	if err != nil {
		s.internalError(w, err)
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
			s.internalError(w, err)
		}
		return
	}
	sid, err := s.s.NewSession(sessionDuration * time.Second)
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.setSessionCookie(w, sid, 2*sessionDuration)
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *server) setSessionCookie(w http.ResponseWriter, sid string, duration int) {
	expires := time.Now().Add(time.Duration(duration) * time.Second)
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Path: "/", Value: sid, MaxAge: duration, Expires: expires, Secure: s.secure})
}

func (s *server) loginPage(w http.ResponseWriter, r *http.Request, path, msg string) {
	err := s.t.ExecuteTemplate(w, "login.html", struct{ Redirect, Message string }{path, msg})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		log.Println(err)
	} else {
		s.s.Remove(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Path: "/", MaxAge: -1, Secure: s.secure})
	path := strings.TrimPrefix(r.URL.Path, "/_/logout")
	if len(path) == len(r.URL.Path) || path == "" {
		path = "/"
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
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

type hostChecker struct {
	hostName  string
	withColon bool
	handler   http.Handler
}

func newHostChecker(hostName string, handler http.Handler) *hostChecker {
	withColon := strings.Index(hostName, ":") >= 0
	return &hostChecker{hostName, withColon, handler}
}

func (hc *hostChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := r.Host
	if hc.withColon {
		if h == hc.hostName {
			hc.handler.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	if h == hc.hostName {
		hc.handler.ServeHTTP(w, r)
	} else {
		http.NotFound(w, r)
	}
}
