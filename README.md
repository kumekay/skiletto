# skiletto

Package manager for agent skills: manifest for intent, lockfile pinned to
commit SHAs, reproducible installs on any machine.

Early development — see the [design doc](https://github.com/kumekay/skiletto/wiki)
for where this is headed.

## Development

Requires Go 1.26+ and system git.

```sh
lefthook install   # gofmt + golangci-lint on commit, tests on push
go test ./...
go build .
```

## License

MIT
