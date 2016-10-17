all: semantic.css

semantic.css: semantic.orig.css main.go
	go run main.go -sel selection.txt semantic.orig.css | \
		sed -e "s/font-family: 'Lato'/font-family: 'LatoWeb'/" \
			-e "s#@import.*family=Lato.*;#@import url('Lato/latofonts.css');#" > semantic.css

toc:
	go run semantic_css_cut.go semantic.orig.css
