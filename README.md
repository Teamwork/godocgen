![Build Status](https://github.com/Teamwork/godocgen/actions/workflows/build.yml/badge.svg)

`godocgen` generates self-contained HTML documentation with `godoc`.

Right now the logic is very much tied to GitHub; although it doesn't *have* to
be. It's just lazyness on my part :-)

Usage
=====

- Edit `config` and fill in your details.

- Generate an access token at https://github.com/settings/tokens (Settings ->
  Developer settings -> Personal access token).
  Only the `public_repo` scope is needed, but for accessing private repos you'll
  need the `repo` one.

  You can also use your regular GitHub password, but I wouldn't recommend it.

- Set your token with `export GITHUB_PASS=`.

- Run `godocgen`.
