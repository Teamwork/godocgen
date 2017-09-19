`godocgen` generates self-contained HTML documentation with `godoc`.

Usage:

1. `git clone git@github.com:Teamwork/godocgen.git && cd godocgen`
2. `git clone git@github.com:Teamwork/docs.git _site`.
3. Edit `config` to suit your needs; specifically, you may want to add some
   repos to a group (e.g. `Libraries` or `Projects`).
4. `go run godocgen.go`.
5. Optional to preview changes: `cd _site && python -mhttp.server`.
6. Push update to `docs`:

		cd _site
		git add -A
		git commit -am 'update'
		git push

7. Now take a moment to contemplate what a sterling job you've done in updating
   the https://tw-godoc.teamwork.com/ site while Travis deploys the update.

Only repositories with the GitHub "language" attribute set to "Go" will be
included. For some repos this gets "guessed" incorrectly (e.g. Desk is "HTML"),
so it also includes repos with the `go` topic.
