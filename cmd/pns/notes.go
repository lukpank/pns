// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/url"
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
	s := n.URL
	q := ""
	if i := strings.IndexByte(s, '?'); i >= 0 {
		q = s[i:]
		s = s[:i]
	}
	if s == "/" {
		s = "/-"
	}
	tags := strings.Split(s[1:], "/")
	if strings.HasPrefix(tag, "/") {
		tags[0] = tag
		return strings.Join(tags, "/") + q
	} else {
		for _, t := range tags[1:] {
			if tag == t {
				return n.URL
			}
		}
		return s + "/" + tag + q
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
	newTags, fts := parseSearchExpr(expr)
	tags := strings.Split(path[1:], "/")
	tags[0] = "/" + tags[0]
	for _, tag := range newTags {
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
	path = strings.Join(tags, "/")
	if fts != "" {
		return path + "?q=" + url.QueryEscape(fts)
	}
	return path
}

func parseSearchExpr(expr string) ([]string, string) {
	const (
		between = iota
		inWord
		inSingleString
		inDoubleString
	)
	var (
		tags, search []string
		state        = between
		start        = 0
	)
	for i, r := range expr {
		switch state {
		case between:
			switch {
			case r == '\'':
				start = i + 1
				state = inSingleString
			case r == '"':
				start = i + 1
				state = inDoubleString
			case !unicode.IsSpace(r):
				start = i
				state = inWord
			}
		case inWord:
			switch {
			case r == '\'':
				tags = append(tags, expr[start:i])
				start = i + 1
				state = inSingleString
			case r == '"':
				tags = append(tags, expr[start:i])
				start = i + 1
				state = inDoubleString
			case unicode.IsSpace(r):
				tags = append(tags, expr[start:i])
				state = between
			}
		case inSingleString:
			if r == '\'' {
				if i > start {
					search = append(search, expr[start:i])
				}
				state = between
			}
		case inDoubleString:
			if r == '"' {
				if i > start {
					s := strings.TrimSpace(expr[start:i])
					if s != "" {
						search = append(search, `"`+s+`"`)
					}
				}
				state = between
			}
		}
	}
	i := len(expr)
	switch state {
	case inWord:
		if i > start {
			tags = append(tags, expr[start:i])
		}
	case inSingleString:
		if i > start {
			search = append(search, expr[start:i])
		}
	case inDoubleString:
		s := strings.TrimSpace(expr[start:i])
		if s != "" {
			search = append(search, `"`+s+`"`)
		}
	}
	return tags, strings.TrimSpace(strings.Join(search, " "))
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

type tagURL struct {
	Name string
	URL  string
}

// ActiveTagsURLs return active topic (if any), active tags and active
// Full text search. The URLs associated with topic, tags and FTS are
// removing given item from the search.
func (n *Notes) ActiveTagsURLs() []tagURL {
	var tagsURLs []tagURL
	s := n.URL[1:]
	q := ""
	if i := strings.IndexByte(s, '?'); i >= 0 {
		q = s[i:]
		s = s[:i]
	}

	// Topic
	tags := strings.Split(s, "/")
	if tags[0] != "" && tags[0] != "-" {
		if len(tags) > 1 {
			i := strings.Index(s, "/")
			tagsURLs = append(tagsURLs, tagURL{"/" + tags[0], "/-" + s[i:] + q})
		} else {
			tagsURLs = append(tagsURLs, tagURL{"/" + tags[0], "/" + q})
		}
	}

	// Tags
	if len(tags) == 2 && tags[0] == "-" {
		tagsURLs = append(tagsURLs, tagURL{tags[1], "/" + q})
	} else {
		for i, tag := range tags[1:] {
			tagsURLs = append(tagsURLs, tagURL{tag, "/" + strings.Join(append(tags[:i+1:i+1], tags[i+2:]...), "/") + q})
		}
	}

	// Full text search
	if len(q) > 1 {
		params := strings.Split(q[1:], "&")
		for i, p := range params {
			if strings.HasPrefix(p, "q=") {
				if s == "-" {
					s = ""
				}
				q := strings.Join(append(params[:i], params[i+1:]...), "&")
				if q != "" {
					q = "?" + q
				}
				u, err := url.QueryUnescape(p[2:])
				if err != nil {
					u = p[2:]
				}
				tagsURLs = append(tagsURLs, tagURL{fmt.Sprintf("'%s'", u), "/" + s + q})
				break
			}
		}
	}

	return tagsURLs
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
