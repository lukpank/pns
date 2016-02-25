// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/golang-commonmark/markdown"
)

const timeLayout = "2006-01-02 15:04:05 -0700"

type Notes struct {
	URL           string
	Notes         []*Note
	md            *markdown.Markdown
	AllTags       []string
	ActiveTags    []string
	AvailableTags []string
	isHTML        bool
	Messages      []string
	Count         int
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

// IDs return slice of IDs of notes to be displayed on a web page used
// for selecting next/previous note using keys on the web page.
func (n *Notes) IDs() []int64 {
	ids := make([]int64, len(n.Notes))
	for i, note := range n.Notes {
		ids[i] = note.ID
	}
	return ids
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

var spacePlusMinus = regexp.MustCompile(`\s*[+-]`)

// tagsURL returns destination URL from a given base URL and
// expression specifying added and removed tags.
func tagsURL(path, expr string) string {
	loc := spacePlusMinus.FindStringIndex(expr)
	if loc != nil {
		if expr[loc[1]-1] == '+' {
			expr = expr[loc[1]:]
		} else {
			expr = expr[loc[1]-1:]
		}
	} else {
		path = "/"
	}
	tags := strings.Split(path[1:], "/")
	tags[0] = "/" + tags[0]
	for _, tag := range strings.Fields(expr) {
		if strings.HasPrefix(tag, "-/") {
			if tags[0] == tag[1:] {
				tags[0] = "/"
			}
		} else if tag[0] == '/' {
			tags[0] = tag
		} else if tag[0] == '-' {
			tags = delTag(tags, tag[1:])
		} else {
			tags = addTag(tags, tag)
		}
	}
	if tags[0] == "/" && len(tags) > 1 {
		tags[0] = "/-"
	}
	return strings.Join(tags, "/")
}

func topicsAndTagsFromEditField(expr string) ([]string, []string) {
	var topics, tags []string
	for _, tag := range strings.Fields(expr) {
		if tag[0] == '-' {
			if strings.HasPrefix(tag, "-/") {
				topics = delTag(topics, tag)
			} else {
				tags = delTag(tags, tag)
			}
		} else if tag[0] == '/' {
			topics = addTag(topics, tag)
		} else {
			tags = addTag(tags, tag)
		}
	}
	return topics, tags
}

func delTag(tags []string, tag string) []string {
	for i, s := range tags {
		if tag == s {
			return append(tags[:i], tags[i+1:]...)
		}
	}
	return tags
}

func addTag(tags []string, tag string) []string {
	for _, s := range tags {
		if s == tag {
			return tags
		}
	}
	return append(tags, tag)
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

func (n *Notes) Render(note *Note) (template.HTML, error) {
	if n.isHTML {
		return template.HTML(note.Text), nil
	}
	var b bytes.Buffer
	err := n.md.Render(&b, []byte(note.Text))
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

func (n *Note) WriteTo(w io.Writer) (int64, error) {
	tags := strings.Join(append(n.Topics, n.Tags...), " ")
	m, err := fmt.Fprintf(w, "%s\n%s\n%s\n%d\n\n%s\n",
		tags, n.Created.Format(timeLayout), n.Modified.Format(timeLayout), n.ID, n.Text)
	return int64(m), err
}

// notesSep returns the shortest slice matching the regular expression
// "[*][*][*]+\s*\n" which does not occur on any of the notes (at the
// begining of a line).
func notesSep(notes []*Note) []byte {
	m := make(map[int]struct{})
	stars := 0
	for _, n := range notes {
		stars = 0
		spaces := false
		for _, r := range n.Text {
			switch {
			case r == '\n':
				if stars >= 3 {
					m[stars] = struct{}{}
				}
				stars = 0
				spaces = false

			case stars < 0:
				continue

			case unicode.IsSpace(r):
				if stars >= 3 {
					spaces = true
				} else {
					stars = -1
				}

			case r == '*' && !spaces:
				stars++

			default:
				stars = -1
			}
		}
	}
	if stars >= 3 {
		m[stars] = struct{}{}
	}
	for i := 3; ; i++ {
		if _, present := m[i]; !present {
			sep := bytes.Repeat([]byte{'*'}, i+1)
			sep[i] = '\n'
			return sep
		}
	}
}

func export(w io.Writer, notes []*Note) error {
	sep := notesSep(notes)
	for _, n := range notes {
		_, err := w.Write(sep)
		if err == nil {
			_, err = n.WriteTo(w)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
