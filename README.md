# Go-TUI
Go-TUI is a simple library for writing text-based user interfaces.
It builds upon tcell's [termbox](https://github.com/ernestrc/tcell) to provide a data model that is simple yet sufficiently expressive to implement classic UNIX TUI programs like less or vi.

# Installation
Install and update this go package with `go get -u github.com/ernestrc/go-tui`.

# Examples
For examples of how to use some of the provided building blocks, see the [./examples](./examples) folder. You can compile them by running `make`, which will compile each of the examples into a running executable in the [./bin](./bin) folder.

# Develop
### Pre-commit hook
Git hooks are installed at [.githooks](.githooks) directory. Run the following to configure git:
```
  git config core.hooksPath .githooks
```
## WebAssembly
This library can be compiled and used in a browser environment:
- Run `make example_wasm` to compile assets into `bin/example_wasm`
- Serve assets with something like `cd bin/example_wasm && goexec 'http.ListenAndServe(":8080", http.FileServer(http.Dir(".")))'` or any other method you prefer.
- Go to `locahost:8080` with your browser.

## Documentation
See https://godoc.org/github.com/ernestrc/go-tui.

# License
Copyright (C) Ernest Romero Climent - All Rights Reserved
Unauthorized copying of the files in this repository, via any medium is strictly prohibited.
Proprietary and confidential.
