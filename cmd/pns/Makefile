GOFILES := $(wildcard *.go)
ASSETSFILES := $(wildcard static/*.css static/*.js static/*.png templates/*.html)
REV := $(shell git log -1 --format=%h)

pns: $(GOFILES)
	go build -ldflags '-X main.Version=pns-0.1-$(REV)'

pnse: $(GOFILES) embedded-assets.go
	go build -o pnse -tags embedded -ldflags '-X main.Version=pns-0.1-$(REV)-embedded' .

embedded-assets.go: $(ASSETSFILES)
	( echo '// +build embedded'; esc -ignore '.*~' static templates) > embedded-assets.go
