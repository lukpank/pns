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
	"strconv"
	"strings"
	"time"

	"github.com/mxk/go-sqlite/sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const queryLimit = 100

type DB struct {
	db  *sql.DB
	git *GitRepo
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
	return &DB{db, NewGitRepo(filename + ".git")}, nil
}

type Querier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func (db *DB) Init(useGit bool, lang string) (err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = createPNSTable(tx, useGit, lang)
	if err == nil {
		_, err = tx.Exec("CREATE TABLE notes(note TEXT, created INTEGER, modified INTEGER)")
	}
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
	return tx.Commit()
}

func createPNSTable(tx *sql.Tx, useGit bool, lang string) error {
	_, err := tx.Exec("CREATE TABLE pns(key TEXT UNIQUE, value TEXT)")
	if err == nil {
		_, err = tx.Exec("INSERT INTO pns (key, value) VALUES ('db_version', '1')")
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO pns (key, value) VALUES ('use_git', ?)", useGit)
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO pns (key, value) VALUES ('lang', ?)", lang)
	}
	return err
}

func (db *DB) getPNSOptions() (git bool, lang string, err error) {
	rows, err := db.db.Query("SELECT key, value FROM pns")
	if err != nil {
		return false, "", err
	}
	defer rows.Close()
	mask := 0
	var key, value string
	for rows.Next() {
		err := rows.Scan(&key, &value)
		if err != nil {
			return false, "", err
		}
		switch key {
		case "db_version":
			mask |= 1
			i, err := strconv.Atoi(value)
			if err != nil {
				return false, "", fmt.Errorf("error parsing db_version: %v", err)
			}
			if i != 1 {
				return false, "", fmt.Errorf("expected db_version 1 but found %d", i)
			}
		case "use_git":
			mask |= 2
			i, err := strconv.Atoi(value)
			if err != nil {
				return false, "", fmt.Errorf("error parsing use_git: %v", err)
			}
			git = i != 0

		case "lang":
			mask |= 4
			lang = value
		}
	}
	if mask&1 == 0 {
		return false, "", errors.New("missing db_version in pns table")
	}
	if mask&2 == 0 {
		return false, "", errors.New("missing use_git in pns table")
	}
	if mask&4 == 0 {
		return false, "", errors.New("missing lang in pns table")
	}
	return
}

func (db *DB) Import(notes []*Note) (err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

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
	return tx.Commit()
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
<h1>{{.Header}}</h1>

<p>
{{range .Tags}}
<a href="{{.}}">{{.}}</a>
{{end}}
</p>
`

var tagsTemplate = template.Must(template.New("tags").Parse(tagsTemplateStr))

const tagsTemplateStr = `
<h1>{{.Header}}</h1>

<p>
{{range .Tags}}
<a href="/-/{{.}}">{{.}}</a>
{{end}}
</p>
`

func (db *DB) TopicsAndTags() ([]string, []string, error) {
	return topicsAndTags(db.db, -1)
}

func (s *server) TopicsAndTagsAsNotes() ([]*Note, []string, error) {
	topics, tags, err := s.db.TopicsAndTags()
	if err != nil {
		return nil, nil, err
	}
	var bTopics, bTags bytes.Buffer
	type data struct {
		Header string
		Tags   []string
	}
	if err = topicsTemplate.Execute(&bTopics, &data{s.tr("Topics"), topics}); err != nil {
		return nil, nil, err
	}
	if err = tagsTemplate.Execute(&bTags, &data{s.tr("Tags"), tags}); err != nil {
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
	defer tx.Rollback()

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
	if err = tx.Commit(); err != nil {
		return nil, err
	}
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

func (db *DB) Notes(topic string, tags []string, fts string, start int, orderedByCreated bool) (notes []*Note, err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if topic != "/-" || len(tags) == 0 {
		tags = append(tags, topic)
	}
	tagIDs, err := db.tagIDs(tx, tags)
	if err != nil {
		return nil, err
	}
	var orderedBy string
	if orderedByCreated {
		orderedBy = fmt.Sprintf("n.created asc LIMIT %d OFFSET %d", queryLimit+1, start)
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
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return notes, nil

}

const ftsQueryFormat = `
SELECT
	rowid, note, created, modified
FROM
	notes
WHERE
        rowid in (SELECT rowid FROM ftsnotes WHERE note MATCH ?)
ORDER BY
        created
LIMIT
	%d
OFFSET
	%d
`

func (db *DB) FTS(q string, start int) ([]*Note, error) {
	tx, err := db.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(fmt.Sprintf(ftsQueryFormat, queryLimit+1, start), q)
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
	if err = tx.Commit(); err != nil {
		return nil, err
	}
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
	defer tx.Rollback()

	// 0. Check sha1sum matches db record
	note, err := db.Note(noteID)
	if err != nil {
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

	// 4. associate new tags with the note
	if len(newIDs) > 0 {
		var args []interface{}
		for _, id := range newIDs {
			args = append(args, noteID, id)
		}
		q = repeatNoLastChar("(?,?),", len(newIDs))
		_, err = tx.Exec(fmt.Sprintf("INSERT INTO tags (noteid, tagid) VALUES %s", q), args...)
		if err != nil {
			return err
		}
	}

	// 5. save to git
	if db.git != nil {
		var b bytes.Buffer
		sort.Strings(tags)
		fmt.Fprintf(&b, "%s\n%s\n\n%s", strings.Join(tags, " "), note.Created.Format(timeLayout), text)
		if err = db.git.Add(idToGitName(noteID), b.Bytes()); err != nil {
			return err
		}
		if err = db.git.Commit(strconv.FormatInt(noteID, 10), now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) addNote(text string, tags []string) (noteID int64, err error) {
	tx, err := db.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

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

	// 4. save to git
	if db.git != nil {
		var b bytes.Buffer
		sort.Strings(tags)
		fmt.Fprintf(&b, "%s\n%s\n\n%s", strings.Join(tags, " "), now.Format(timeLayout), text)
		if err = db.git.Add(idToGitName(noteID), b.Bytes()); err != nil {
			return 0, err
		}
		if err = db.git.Commit(strconv.FormatInt(noteID, 10), now); err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
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
