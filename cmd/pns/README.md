PNS (Personal note server)
==========================

Before starting the server you have to initialize the database
file. This is done with

```
$ pns -f filename.db -init
```

Next you can import data into the database with

```
$ pns -f filename.db -import input.md
```

And add a user with

```
$ pns -f filename.db -adduser login
```

Later, if you want to export all notes from the database use

```
$ pns -f filename.db -export / -o output.md
```

Or you can export notes matching a filter of the form
`/topic/tag1/.../tagn`, where topic may be `-` for given tags on all
topics.

You can use `-init`, `-import` and `-adduser` (and even `-export`) in
a single command. They are executed in this order.

Then you can start serving HTTP with

```
$ pns -f filename.db -http :8080 -host your.host.domain.name
```

where we (optionally) specify hostname which must be used in the
request, otherwise "404 not found" will be returned (this is a simple
countermeasure against evil crawlers using random IP numbers).

Or better use HTTPS with

```
pns -f test.db -https :8080 -https_cert cert.pem -https_key key.pem -host your.host.domain.name
```


Keyboard navigation
-------------------

PNS can be navigated using keyboard shortcuts.

On the pages with notes the following shortcuts are available

| Key     | Action
|---------|----------------------------------------------------------------------------------------------
| `Alt+n` | select next note (just `n` will work if some note is already selected)
| `Alt+p` | select previous note (just `p` will work if some note is already selected)
| `Alt+l` | switch between tag editing field and current note (just `l` if not in tag editing field)
| `Alt+a` | add new note (just `a` will work if tag editing field is not selected; as "Add note" button)
| `e`     | edit selected note (as "Edit" link)


On the note editing/adding pages the following shortcuts are available

| Key     | Action
|---------|------------------------------------------------------
| `Alt+l` | switch between tag editing field and note text field
| `Alt+q` | quit editing note (as "Cancel" button)
| `Alt+r` | reload preview (as "Preview" button)
| `Alt+s` | submit note (as "Submit" button)


Upgrade database
----------------

### before commit "add two indexes on tags table"

To upgrade database which does not have indices (created before
"add two indexes on tags table" commit) start sqlite with the database
as argument and add indexes with

```
CREATE UNIQUE INDEX tagsIds ON tags (noteid, tagid);
CREATE INDEX tagsTagId ON tags (tagid);
```

You can check the names of defined indexes with sqlite command

```
.indexes
```

### before commit "add full text search"

To upgrade database which does not have full text search table
(created before "add full text search" commit) start sqlite with the
database as argument and add and populate full text index with

```
CREATE VIRTUAL TABLE ftsnotes USING fts4(note);
INSERT INTO ftsnotes(docid, note) SELECT rowid, note FROM notes;
```
