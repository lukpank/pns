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
$ pns -f filename.db -http :8080
```
