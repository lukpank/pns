// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"html/template"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-commonmark/markdown"
)

func main() {
	t, err := template.New("layout").Parse(layout)
	if err != nil {
		log.Fatal(err)
	}
	err = t.Execute(os.Stdout, &Notes{"/test", notes, markdown.New()})
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
	ID       int
	Text     string
}

var notes = []*Note{
	{
		Topics: []string{
			"/test", "/test2",
		},
		Tags: []string{
			"foo", "bar",
		},
		Created:  time.Now(),
		Modified: time.Now(),
		ID:       1,
		Text:     "This is a **test**.",
	},

	{
		Topics: []string{
			"/test",
		},
		Tags: []string{
			"foo", "baz",
		},
		Created:  time.Now(),
		Modified: time.Now(),
		ID:       2,
		Text:     "This is another **test**.",
	},
}

const layout = `
<!DOCTYPE html>
<html>
<head>
<link type="text/css" rel="stylesheet" href="style.css">
</head>

<body>
{{range $n := .Notes}}
<div class="note">
{{$.Render .Text}}

<div class="note-footer">

{{range .Topics}}
<a href="{{$.TagURL .}}" class="tag">
{{.}}
</a>
{{end}}

{{range .Tags}}
<a href="{{$.TagURL .}}" class="tag">
{{.}}
</a>
{{end}}

<span class="edit">
<a href="/_/edit/{{.ID}}" class="tag">
Edit
</a>
</span>

<span class="time">
{{.Modified.Format "2006-01-02 15:04:05 -0700"}}
</span>

</div>
</div>
{{end}}
</body>
</html>
`
