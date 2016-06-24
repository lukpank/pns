all: semantic.css

semantic.css: semantic.orig.css main.go
	go run main.go -sel selection.txt -o semantic.css semantic.orig.css 

toc:
	go run semantic_css_cut.go semantic.orig.css
