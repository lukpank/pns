// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/golang-commonmark/markdown"
)

type Notes struct {
	URL           string
	Notes         []*Note
	md            *markdown.Markdown
	availableTags []string
	isHTML        bool
	Messages      []string
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

func (n *Notes) AvailableTags() string {
	return strings.Join(n.availableTags, ", ")
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

// tagsURL returns destination URL from a given base URL and
// expression specifying added and removed tags.
func tagsURL(path, expr string) string {
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
