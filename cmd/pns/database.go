// Copyright 2016 Łukasz Pankowski <lukpank at o2 dot pl>. All rights
// reserved.  This source code is licensed under the terms of the MIT
// license. See LICENSE file for details.

package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/mxk/go-sqlite/sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	db *sql.DB
}

var (
	ErrSingleThread = errors.New("single threaded sqlite3 is not supported")
	ErrTagName      = errors.New("unexpected tag name in query result")
	ErrAuth         = errors.New("failed to authenticate: incorect login or password")
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

type Querier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
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
	if err == nil {
		_, err = tx.Exec("CREATE TABLE users(login TEXT UNIQUE, passwordhash BLOB)")
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

func (db *DB) AddUser(login string, password []byte) error {
	p, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.db.Exec("INSERT INTO users (login, passwordhash) VALUES (?, ?)", login, p)
	return err
}

func (db *DB) AuthenticateUser(login string, password []byte) error {
	var h []byte
	if err := db.db.QueryRow("SELECT passwordhash FROM users WHERE login=?", login).Scan(&h); err != nil {
		if err == sql.ErrNoRows {
			return ErrAuth
		}
		return err
	}
	err := bcrypt.CompareHashAndPassword(h, password)
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return ErrAuth
	}
	return err
}

var topicsTemplate = template.Must(template.New("topics").Parse(topicsTemplateStr))

const topicsTemplateStr = `
<h1>Topics</h1>

<p>
{{range .}}
<a href="{{.}}">{{.}}</a>
{{end}}
</p>
`

var tagsTemplate = template.Must(template.New("tags").Parse(tagsTemplateStr))

const tagsTemplateStr = `
<h1>Tags</h1>

<p>
{{range .}}
<a href="/-/{{.}}">{{.}}</a>
{{end}}
</p>
`

func (db *DB) TopicsAndTags() ([]string, []string, error) {
	return topicsAndTags(db.db, -1)
}

func (db *DB) TopicsAndTagsAsNotes() ([]*Note, []string, error) {
	topics, tags, err := db.TopicsAndTags()
	if err != nil {
		return nil, nil, err
	}
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

// NewTags for given list of tags and topics returns those that are
// not found in the database.
func (db *DB) NewTags(tags []string) ([]string, error) {
	// select rowid, * from tagnames where name in ("db", "todo", "spec");
	var args []interface{}
	for _, tag := range tags {
		args = append(args, tag)
	}
	query := fmt.Sprintf("SELECT name FROM tagnames WHERE name IN (%s)", questionMarks(len(tags)))
	rows, err := db.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		m[name] = struct{}{}
	}
	var newTags []string
	for _, tag := range tags {
		if _, present := m[tag]; !present {
			newTags = append(newTags, tag)
		}
	}
	sort.Strings(newTags)
	return newTags, nil
}

// Note returns note with the given ID
func (db *DB) Note(id int64) (*Note, error) {
	var note string
	var created, modified int64
	err := db.db.QueryRow("SELECT note, created, modified FROM notes WHERE rowid=?", id).Scan(&note, &created, &modified)
	if err != nil {
		return nil, err
	}
	topics, tags, err := topicsAndTags(db.db, id)
	if err != nil {
		return nil, err
	}
	return &Note{ID: id, Text: note, Created: time.Unix(created, 0), Modified: time.Unix(modified, 0),
		Topics: topics, Tags: tags}, nil
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
	query := fmt.Sprintf(notesQueryFormat, questionMarks(len(tagIDs)))
	rows, err := tx.Query(query, append(tagIDs, len(tagIDs))...)
	if err != nil {
		return nil, err
	}
	notes, err := notesFromRowsClose(rows)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		n.Topics, n.Tags, err = topicsAndTags(tx, n.ID)
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
	// tags may have duplicated tags, on tagsUnique we take only unique ones
	tagsUnique := make([]interface{}, 0)
	for tag := range m {
		tagsUnique = append(tagsUnique, tag)
	}
	q := questionMarks(len(tagsUnique))
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
	if len(m) != len(tagsUnique) {
		return nil, ErrTagName
	}
	if len(ids) != len(tagsUnique) {
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

func topicsAndTags(tx Querier, noteID int64) (topics, tags []string, err error) {
	var rows *sql.Rows
	if noteID < 0 {
		rows, err = tx.Query("SELECT name FROM tagnames")
	} else {
		rows, err = tx.Query(topicsAndTagsQuery, noteID)
	}
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, nil, err
		}
		if len(tag) > 0 && tag[0] == '/' {
			topics = append(topics, tag)
		} else {
			tags = append(tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	sort.Strings(topics)
	sort.Strings(tags)
	return
}

const topicsAndTagsQuery = `
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

// questionMarks returns cnt comma separated question marks to be used
// in a query string with varying number of arguments.
func questionMarks(cnt int) string {
	return strings.Repeat("?,", cnt)[:2*cnt-1]
}
