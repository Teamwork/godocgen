`godocgen` generates self-contained HTML documentation with `godoc`.

Usage:

1. `git clone git@github.com:Teamwork/docs.git _site`.
2. Edit `config` to suit your needs; specifically, you may want to add some
   repos to a group (e.g. `Libraries` or `Projects`).
3. `go run godocgen.go`.
4. Optional to preview changes: `cd _site && python -mhttp.server`.
5. Push update to `docs`:
   `cd _site && git add -A && t clone git@github.com:Teamwork/doc.git _sitegit commit -am 'update' && git push`
4. Now take a moment to contemplate what a sterling job you've done in updating
   the https://tw-godoc.teamwork.com/ site while Travis deploys the update.

Only repositories with the GitHub "language" attribute set to "Go" will be
included. For some repos this gets "guessed" incorrectly (e.g. Desk is "HTML"),
so it also includes repos with the `go` topic.
