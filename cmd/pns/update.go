// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const maxInt = int(^uint(0) >> 1)

func updateDB(db *DB, filename string, useGit bool, lang string) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = createPNSTable(tx, useGit, lang)
	if err != nil {
		return err
	}
	if !useGit {
		return tx.Commit()
	}
	notes, err := db.AllNotes()
	if err != nil {
		return err
	}
	g := NewGitRepo(filename + ".git")
	if err := g.Init(); err != nil {
		return err
	}
	ref, first, err := g.getHEAD()
	if err != nil {
		return err
	}
	if !first {
		return errors.New("git: unexpected commits in fresh created repository")
	}

	var b bytes.Buffer
	var parent SHA1
	p := NewProgress(len(notes))
	for i, n := range notes {
		tags := strings.Join(append(n.Topics, n.Tags...), " ")
		b.Reset()
		fmt.Fprintf(&b, "%s\n%s\n\n%s", tags, n.Created.Format(timeLayout), n.Text)

		h, err := g.hashObject(objectBlob, b.Bytes())
		if err != nil {
			return err
		}
		if n.ID < 0 {
			return fmt.Errorf("unsupported ID=%d, is negative", n.ID)
		}
		if n.ID > int64(maxInt) {
			return fmt.Errorf("unsupported, ID=%d exceeds size of int", n.ID)
		}
		g.addToIndex(idToGitName(n.ID), h)
		h, err = g.addWriteTree(int(n.ID), h)
		if err != nil {
			return err
		}
		parent, err = g.writeCommit(int(n.ID), h, parent, n.Modified)
		if err != nil {
			return err
		}
		if i%5000 == 4999 || i == len(notes)-1 {
			if err := g.updateRef(ref, hex.EncodeToString(parent[:])); err != nil {
				return err
			}
			if err := g.updateIndex(); err != nil {
				return err
			}
			os.Stderr.WriteString("\n")
			if err := g.GC(); err != nil {
				return err
			}
		}
		p.Done()
	}
	return tx.Commit()
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
