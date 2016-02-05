// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-commonmark/markdown"
)

func main() {
	var notes []*Note
	var err error
	if len(os.Args) > 1 {
		notes, err = parseFile(os.Args[1])
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
	notes2, err := db.Notes("/pns", []string{"db"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintln(os.Stderr, len(notes2))
	t, err := template.New("layout").Parse(layout)
	if err != nil {
		log.Fatal(err)
	}
	var out io.Writer
	if len(os.Args) > 2 {
		out, err = os.Create(os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
	} else {
		out = os.Stdout
	}
	err = t.Execute(out, &Notes{"/test", notes, markdown.New()})
	if err != nil {
		log.Fatal(err)
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
}

const layout = `
<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8">
<link type="text/css" rel="stylesheet" href="style.css">
</head>

<body>

<div class="topbar">
<ul>
<li><a href="/-">/pns</a></li>
<li><a href="/pns">delme</a></li>
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
</div>
{{end}}

</div>
</body>
</html>
`
