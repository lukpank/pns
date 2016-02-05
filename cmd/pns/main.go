// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
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
	t, err := template.New("layout").Parse(layout)
	if err != nil {
		log.Fatal(err)
	}
	if len(args) > 1 {
		out, err := os.Create(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		err = t.Execute(out, &Notes{"/test", notes, markdown.New()})
		if err != nil {
			log.Fatal(err)
		}
	} else {
		http.Handle("/", &server{db, t, markdown.New()})
		http.Handle("/_/static/", http.StripPrefix("/_/static/", http.FileServer(http.Dir("./static/"))))
		log.Fatal(http.ListenAndServe(*httpAddr, nil))
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
	var notes []*Note
	var err error
	if path == "/" || path == "/-" {
		notes, err = s.db.TopicsAndTags()
	} else {
		notes, err = s.db.Notes("/"+tags[1], tags[2:])
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = s.t.Execute(w, &Notes{path, notes, s.md})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type Notes struct {
	URL   string
	Notes []*Note
	md    *markdown.Markdown
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

func (n *Notes) ShowTopic() bool {
	return n.URL != "/" && !strings.HasPrefix(n.URL, "/-")
}

func (n *Notes) AllTopicURL() string {
	s := n.URL[1:]
	i := strings.Index(s, "/")
	if i < 0 {
		return "/"
	} else {
		return "/-" + s[i:]
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

type Note struct {
	Topics   []string
	Tags     []string
	Created  time.Time
	Modified time.Time
	ID       int64
	Text     string
	NoFooter bool
}

const layout = `
<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
<link type="text/css" rel="stylesheet" href="/_/static/style.css">
</head>

<body>

<div class="topbar">
<ul>
<li>{{if .ShowTopic}}<a href="{{$.AllTopicURL}}">/pns</a>{{else}}&nbsp;{{end}}</li>
{{range .Tags}}
<li><a href="{{$.DelTagURL .}}">{{.}}</a></li>
{{end}}
</ul>
<form>
<input type="text" class="taginput" placeholder="Add tag"></input>
</form>
</div>

<div class="content">

{{range $n := .Notes}}
<a id="{{.ID}}" class="anchor"></a>
<div class="note">
{{$.Render .Text}}

{{if (not .NoFooter)}}
<div class="note-footer">

{{range .Topics}}
<a href="{{$.TagURL .}}" class="tag">{{.}}</a> ·
{{end}}

{{range .Tags}}
<a href="{{$.TagURL .}}" class="tag">{{.}}</a> ·
{{end}}

<span class="tag">
{{.Modified.Format "2006-01-02 15:04:05 -0700"}}
</span> ·

<span class="tag">
<a href="/_/edit/{{.ID}}" class="tag">Edit</a>
</span> ·

<span class="tag">
<a href="#{{.ID}}" class="tag">#</a>
</span>

</div>
{{end}}

</div>
{{end}}

</div>
</body>
</html>
`
