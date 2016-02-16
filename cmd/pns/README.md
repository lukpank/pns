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

You can use `-init`, `-import` and `-adduser` in a single
command. They are executed in this order.

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
