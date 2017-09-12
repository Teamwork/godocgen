`godocgen` generates self-contained HTML documentation with `godoc`.

Run `gen`. This may take some time the first time 'round since it will clone all
our Go repos in `_clone`. This script will only clone repos with the "language"
set to "Go". For some repos this gets "guessed" incorrectly (e.g. Desk is "HTML"
according to GitHub), so it also includes repos with the `go` topic.

You will need `go` and `jq`.

To run it locally use `cd _site && python -mhttp.server`.

To deploy use `cd _site && netlify deploy -s designer-herbert-47871 -p .`.
