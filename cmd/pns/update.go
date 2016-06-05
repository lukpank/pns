// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func updateDB(db *DB, filename string, git bool) error {
	if !git {
		return nil
	}
	notes, err := db.AllNotes()
	if err != nil {
		return err
	}
	g := NewGitRepo(filename + ".git")
	if err := g.Init(); err != nil {
		return err
	}
	var b bytes.Buffer
	p := NewProgress(len(notes))
	for i, n := range notes {
		tags := strings.Join(append(n.Topics, n.Tags...), " ")
		b.Reset()
		fmt.Fprintf(&b, "%s\n%s\n\n%s", tags, n.Created.Format(timeLayout), n.Text)
		name := idToGitName(n.ID)
		if err := g.Add(name, b.Bytes()); err != nil {
			return err
		}
		if err := g.Commit(strconv.FormatInt(n.ID, 10), n.Modified); err != nil {
			return err
		}
		if i%5000 == 4999 || i == len(notes)-1 {
			os.Stderr.WriteString("\n")
			if err = g.GC(); err != nil {
				return err
			}
		}
		p.Done()
	}
	return nil
}

func idToGitName(id int64) string {
	s := strconv.FormatInt(id, 10)
	if len(s)&1 == 1 {
		s = "0" + s
	}
	var b bytes.Buffer
	if len(s) == 2 {
		b.WriteString("00/")
	}
	for i := 0; i < len(s); i += 2 {
		if i > 0 {
			b.WriteByte('/')
		}
		b.WriteString(s[i : i+2])
	}
	b.WriteString(".md")
	return b.String()
}
