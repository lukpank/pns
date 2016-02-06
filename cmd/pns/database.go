// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/mxk/go-sqlite/sqlite3"
)

type DB struct {
	db *sql.DB
}

var (
	ErrSingleThread = errors.New("single threaded sqlite3 is not supported")
	ErrTagName      = errors.New("unexpected tag name in query result")
)

func OpenDB(filename string) (*DB, error) {
	if sqlite3.SingleThread() {
		return nil, ErrSingleThread
	}
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	return &DB{db}, nil
}

func (db *DB) Init() (err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)
	_, err = tx.Exec("CREATE TABLE notes(note TEXT, created INTEGER, modified INTEGER)")
	if err == nil {
		_, err = tx.Exec("CREATE TABLE tags(noteid INTEGER, tagid INTEGER)")
	}
	if err == nil {
		_, err = tx.Exec("CREATE TABLE tagnames(name TEXT UNIQUE)")
	}
	if err != nil {
		return err
	}
	done = true
	return nil
}

func commitOrRollback(tx *sql.Tx, done *bool, err *error) {
	var e error
	if *done {
		e = tx.Commit()
	} else {
		e = tx.Rollback()
	}
	if *err != nil {
		if e != nil {
			*err = MultiError{*err, e}
		}
	} else {
		*err = e
	}
}

func (db *DB) Import(notes []*Note) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)

	m := make(map[string]int64)
	for _, n := range notes {
		for _, s := range n.Topics {
			m[s] = -1
		}
		for _, s := range n.Tags {
			m[s] = -1
		}
	}

	for k := range m {
		result, err := tx.Exec("INSERT INTO tagnames VALUES(?)", k)
		if err != nil {
			return err
		}
		m[k], err = result.LastInsertId()
		if err != nil {
			return err
		}
	}

	for _, n := range notes {
		result, err := tx.Exec("INSERT INTO notes (note, created, modified) VALUES(?, ?, ?)",
			n.Text, n.Created, n.Modified)
		if err != nil {
			return err
		}
		noteid, err := result.LastInsertId()
		if err != nil {
			return err
		}

		for _, s := range n.Topics {
			_, err := tx.Exec("INSERT INTO tags (noteid, tagid) VALUES(?, ?)", noteid, m[s])
			if err != nil {
				return err
			}
		}
		for _, s := range n.Tags {
			_, err := tx.Exec("INSERT INTO tags (noteid, tagid) VALUES(?, ?)", noteid, m[s])
			if err != nil {
				return err
			}
		}
	}
	done = true
	return nil
}

var topicsTemplate = template.Must(template.New("topics").Parse(topicsTemplateStr))

const topicsTemplateStr = `
# Topics

{{range .}}[{{.}}]({{.}}) {{end}}
`

var tagsTemplate = template.Must(template.New("tags").Parse(tagsTemplateStr))

const tagsTemplateStr = `
# Tags

{{range .}}[{{.}}](/-/{{.}}) {{end}}
`

func (db *DB) TopicsAndTags() ([]*Note, []string, error) {
	rows, err := db.db.Query("SELECT name FROM tagnames")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var topics, tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, nil, err
		}
		if len(tag) > 0 && tag[0] == '/' {
			if validTag(tag[1:]) {
				topics = append(topics, tag)
			}
		} else {
			if validTag(tag) {
				tags = append(tags, tag)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	sort.Strings(topics)
	sort.Strings(tags)
	var bTopics, bTags bytes.Buffer
	if err = topicsTemplate.Execute(&bTopics, topics); err != nil {
		return nil, nil, err
	}
	if err = tagsTemplate.Execute(&bTags, tags); err != nil {
		return nil, nil, err
	}
	notes := []*Note{
		{Text: bTopics.String(), NoFooter: true},
		{Text: bTags.String(), NoFooter: true},
	}
	return notes, append(topics, tags...), nil
}

func validTag(tag string) bool {
	for _, r := range tag {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func (db *DB) Notes(topic string, tags []string) ([]*Note, error) {
	tx, err := db.db.Begin()
	if err != nil {
		return nil, err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)

	if topic != "/-" || len(tags) == 0 {
		tags = append(tags, topic)
	}
	tagIDs, err := db.tagIDs(tx, tags)
	if err != nil {
		return nil, err
	}
	q := strings.Repeat("?,", len(tagIDs))[:2*len(tagIDs)-1]
	rows, err := tx.Query(fmt.Sprintf(notesQueryFormat, q), append(tagIDs, len(tagIDs))...)
	if err != nil {
		return nil, err
	}
	notes, err := notesFromRowsClose(rows)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		addTags(tx, n)
	}
	done = true
	return notes, nil

}

const notesQueryFormat = `
SELECT
	n.rowid,
	n.note,
	n.created,
	n.modified
FROM
	notes AS n
INNER JOIN
	tags AS t
ON
	n.rowid = t.noteid
AND
	t.tagid in (%s)
GROUP BY
	n.rowid
HAVING
	COUNT(n.rowid)=?
ORDER BY
	n.created asc
`

func (db *DB) tagIDs(tx *sql.Tx, tags []string) ([]interface{}, error) {
	m := make(map[string]bool)
	for _, tag := range tags {
		m[tag] = false
	}
	// tags may have duplicated tags, on tagUnique we take only unique ones
	tagsUnique := make([]interface{}, 0)
	for tag := range m {
		tagsUnique = append(tagsUnique, tag)
	}
	q := strings.Repeat("?,", len(tags))[:2*len(tags)-1]
	rows, err := tx.Query(fmt.Sprintf("SELECT rowid, name from tagnames where name in (%s)", q), tagsUnique...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]interface{}, 0, len(tags))
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		ids = append(ids, id)
		m[name] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(m) != len(tags) {
		return nil, ErrTagName
	}
	if len(ids) != len(tags) {
		var err NoTagsError
		for s, present := range m {
			if !present {
				err = append(err, s)
			}
		}
		return nil, err
	}
	return ids, nil
}

func notesFromRowsClose(rows *sql.Rows) ([]*Note, error) {
	defer rows.Close()

	var notes []*Note
	for rows.Next() {
		var note string
		var rowid, created, modified int64
		if err := rows.Scan(&rowid, &note, &created, &modified); err != nil {
			return nil, err
		}
		notes = append(notes, &Note{ID: rowid, Text: note, Created: time.Unix(created, 0), Modified: time.Unix(modified, 0)})

	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return notes, nil
}

func addTags(tx *sql.Tx, n *Note) error {
	rows, err := tx.Query(addTagsQuery, n.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return err
		}
		if len(tag) > 0 && tag[0] == '/' {
			n.Topics = append(n.Topics, tag)
		} else {
			n.Tags = append(n.Tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	sort.Strings(n.Topics)
	sort.Strings(n.Tags)
	return nil
}

const addTagsQuery = `
SELECT
	n.name
FROM
	tags AS t
INNER JOIN
	tagnames AS n
ON
	t.noteid=?
AND
	t.tagid = n.rowid
`

type MultiError []error

func (me MultiError) Error() string {
	var s []string
	for _, err := range me {
		s = append(s, err.Error())
	}
	return strings.Join(s, "; ")
}

type NoTagsError []string

func (e NoTagsError) Error() string {
	return "no such tags: " + strings.Join(e, ", ")
}
