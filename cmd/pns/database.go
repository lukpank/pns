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
	ErrAuth         = errors.New("failed to authenticate")
	ErrNoTags       = errors.New("no tags specified")
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
		_, err = tx.Exec("CREATE VIRTUAL TABLE ftsnotes USING fts4(note)")
	}
	if err == nil {
		_, err = tx.Exec("CREATE TABLE tags(noteid INTEGER, tagid INTEGER)")
	}
	if err == nil {
		_, err = tx.Exec("CREATE UNIQUE INDEX tagsIds ON tags (noteid, tagid)")
	}
	if err == nil {
		_, err = tx.Exec("CREATE INDEX tagsTagId ON tags (tagid)")
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

func (db *DB) Import(notes []*Note) (err error) {
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
		_, err = tx.Exec("INSERT INTO ftsnotes (docid, note) VALUES (?, ?)", noteid, n.Text)
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
	query := fmt.Sprintf("SELECT name FROM tagnames WHERE name IN (%s)", questionMarks(len(tags)))
	rows, err := db.db.Query(query, stringsAsEmptyInterface(tags)...)
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

func stringsAsEmptyInterface(input []string) (output []interface{}) {
	for _, s := range input {
		output = append(output, s)
	}
	return
}

func int64sAsEmptyInterface(input []int64) (output []interface{}) {
	for _, s := range input {
		output = append(output, s)
	}
	return
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

func (db *DB) AllNotes() (notes []*Note, err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return nil, err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)

	rows, err := tx.Query("SELECT rowid, note, created, modified FROM notes ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	notes, err = notesFromRowsClose(rows)
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
	%s
`

const notesQueryWithFtsFormat = `
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
AND
	n.rowid in (SELECT rowid FROM ftsnotes WHERE note MATCH ?)
GROUP BY
	n.rowid
HAVING
	COUNT(n.rowid)=?
ORDER BY
	%s
`

func (db *DB) Notes(topic string, tags []string, fts string, orderedByCreated bool) (notes []*Note, err error) {
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
	var orderedBy string
	if orderedByCreated {
		orderedBy = "n.created asc"
	} else {
		orderedBy = "n.rowid asc"
	}
	var (
		query string
		args  []interface{}
	)
	if fts != "" {
		query = fmt.Sprintf(notesQueryWithFtsFormat, questionMarks(len(tagIDs)), orderedBy)
		args = append(tagIDs, fts, len(tagIDs))
	} else {
		query = fmt.Sprintf(notesQueryFormat, questionMarks(len(tagIDs)), orderedBy)
		args = append(tagIDs, len(tagIDs))
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	notes, err = notesFromRowsClose(rows)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		n.Topics, n.Tags, err = topicsAndTags(tx, n.ID)
	}
	done = true
	return notes, nil

}

const ftsQuery = `
SELECT
	rowid, note, created, modified
FROM
	notes
WHERE
        rowid in (SELECT rowid FROM ftsnotes WHERE note MATCH ?)
`

func (db *DB) FTS(q string) ([]*Note, error) {
	tx, err := db.db.Begin()
	if err != nil {
		return nil, err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)

	rows, err := tx.Query(ftsQuery, q)
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

// tagsToIDsMayInsert returns slice of tag IDs corresponding to given
// tag (and topic) names. Those tag names which are not in the
// database are inserted into tagnames table and such obtained tag IDs
// are returned. tagsToIDsMayInsert can deal with duplicated tags.
// An empty tag list is considered an error.
func (db *DB) tagsToIDsMayInsert(tx *sql.Tx, tags []string) ([]int64, error) {
	if len(tags) == 0 {
		return nil, ErrNoTags
	}
	q := questionMarks(len(tags))
	rows, err := tx.Query(fmt.Sprintf("SELECT rowid, name FROM tagnames WHERE name IN (%s)", q), stringsAsEmptyInterface(tags)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]struct{})
	ids := make([]int64, 0, len(tags))
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		ids = append(ids, id)
		m[name] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	newNames := make([]string, 0, len(tags))
	for _, s := range tags {
		if _, present := m[s]; !present {
			newNames = append(newNames, s)
		}
		m[s] = struct{}{}
	}
	if len(newNames) == 0 {
		return ids, nil
	}

	for _, s := range newNames {
		result, err := tx.Exec("INSERT INTO tagnames (name) VALUES (?)", s)
		if err != nil {
			return nil, err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (db *DB) updateNote(noteID int64, text string, tags []string, sha1sum string) (err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)

	// 0. Check sha1sum matches db record
	if note, err := db.Note(noteID); err != nil {
		return err
	} else {
		dbSHA1Sum := note.sha1sum()
		if dbSHA1Sum != sha1sum {
			return &EditConflictError{dbSHA1Sum}
		}
	}

	// 1. Update note.
	now := time.Now()
	_, err = tx.Exec("UPDATE notes SET note=?, modified=? where rowid=?", text, now, noteID)
	if err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE ftsnotes SET note=? WHERE rowid=?", text, noteID)
	if err != nil {
		return err
	}

	// 2. get tag IDs
	ids, err := db.tagsToIDsMayInsert(tx, tags)
	if err != nil {
		return err
	}

	// 3. Delete tags no longer associated with the note
	q := questionMarks(len(ids))
	query := fmt.Sprintf("DELETE FROM tags WHERE noteid=? AND tagid NOT IN (%s)", q)
	_, err = tx.Exec(query, append([]interface{}{noteID}, int64sAsEmptyInterface(ids)...)...)
	if err != nil {
		return err
	}

	// 3. leave only those tags which are not yet associated with the note
	rows, err := tx.Query("SELECT tagid FROM tags WHERE noteid=?", noteID)
	if err != nil {
		return err
	}
	m := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		m[id] = struct{}{}
	}
	newIDs := make([]int64, 0, len(ids)-len(m))
	for _, id := range ids {
		if _, present := m[id]; !present {
			newIDs = append(newIDs, id)
		}
	}
	if len(newIDs) == 0 {
		done = true
		return nil
	}

	// 4. associate new tags with the note
	var args []interface{}
	for _, id := range newIDs {
		args = append(args, noteID, id)
	}
	q = repeatNoLastChar("(?,?),", len(newIDs))
	_, err = tx.Exec(fmt.Sprintf("INSERT INTO tags (noteid, tagid) VALUES %s", q), args...)
	if err != nil {
		return err
	}

	done = true
	return nil
}

func (db *DB) addNote(text string, tags []string) (noteID int64, err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return 0, err
	}
	done := false
	defer commitOrRollback(tx, &done, &err)

	// 1. Update note.
	now := time.Now()
	result, err := tx.Exec("INSERT INTO notes (note, created, modified) VALUES (?, ?, ?)", text, now, now)
	if err != nil {
		return 0, err
	}
	noteID, err = result.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec("INSERT INTO ftsnotes (docid, note) VALUES (?, ?)", noteID, text)
	if err != nil {
		return 0, err
	}

	// 2. get tag IDs
	ids, err := db.tagsToIDsMayInsert(tx, tags)
	if err != nil {
		return 0, err
	}

	// 3. associate tags with the note
	var args []interface{}
	for _, id := range ids {
		args = append(args, noteID, id)
	}
	q := repeatNoLastChar("(?,?),", len(ids))
	_, err = tx.Exec(fmt.Sprintf("INSERT INTO tags (noteid, tagid) VALUES %s", q), args...)
	if err != nil {
		return 0, err
	}

	done = true
	return
}

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

type EditConflictError struct {
	SHA1Sum string // sha1sum of note in the DB
}

func (e *EditConflictError) Error() string {
	return "conflicting edit detected"
}

// questionMarks returns cnt comma separated question marks to be used
// in a query string with varying number of arguments.
func questionMarks(cnt int) string {
	return strings.Repeat("?,", cnt)[:2*cnt-1]
}

func repeatNoLastChar(s string, cnt int) string {
	t := strings.Repeat(s, cnt)
	return t[:len(t)-1]
}
