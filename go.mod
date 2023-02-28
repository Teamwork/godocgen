module github.com/teamwork/godoc

go 1.19

require (
	github.com/PuerkitoBio/goquery v1.8.0
	github.com/andybalholm/cascadia v1.3.1
	github.com/arp242/hubhub v0.0.0-20211017231734-35ca2b2ed259
	github.com/arp242/sconfig v1.2.2
	github.com/arp242/singlepage v1.0.0
	github.com/pkg/errors v0.8.0
	github.com/tdewolff/minify v2.3.5+incompatible
	github.com/tdewolff/parse v2.3.3+incompatible
	github.com/teamwork/utils v0.0.0-20180828160709-681764439846
	golang.org/x/net v0.0.0-20210916014120-12bc252f5db8
)

require (
	github.com/tdewolff/minify/v2 v2.10.0 // indirect
	github.com/tdewolff/parse/v2 v2.5.27 // indirect
	zgo.at/zstd v0.0.0-20220306174247-aa79e904bd64 // indirect
)

replace (
	github.com/arp242/hubhub v0.0.0-20211017231734-35ca2b2ed259 => zgo.at/hubhub v0.0.0-20211017231734-35ca2b2ed259
	github.com/arp242/sconfig v1.2.2 => zgo.at/sconfig v1.2.2
	github.com/arp242/singlepage v1.0.0 => zgo.at/singlepage v1.0.0
)
