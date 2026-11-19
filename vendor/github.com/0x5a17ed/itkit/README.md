# itkit

[![Go Reference](https://pkg.go.dev/badge/github.com/0x5a17ed/itkit.svg)](https://pkg.go.dev/github.com/0x5a17ed/itkit)
[![License: APACHE-2.0](https://img.shields.io/badge/license-APACHE--2.0-blue?style=flat-square)](https://www.apache.org/licenses/)
[![Go Report Card](https://goreportcard.com/badge/github.com/0x5a17ed/itkit?style=flat-square)](https://goreportcard.com/report/github.com/0x5a17ed/itkit)
[![codecov](https://img.shields.io/codecov/c/gh/0x5a17ed/itkit?style=flat-square)](https://codecov.io/gh/0x5a17ed/itkit)

Short, dead simple and concise generic iterator interface. With a few extras similar to what python has to offer.


## 📦 Installation

```shell
$ go get -u github.com/0x5a17ed/itkit@latest
```


## 🤔 Usage

```go
package main

import (
	"fmt"

	"github.com/0x5a17ed/itkit/iters/sliceit"
	"github.com/0x5a17ed/itkit/itlib"
)

func main() {
	s := []int{1, 2, 3}

	// iterating using the for keyword.
	for it := sliceit.In(s); it.Next(); {
		fmt.Println(it.Value())
	}

	// iterating using a slightly more functional approach.
	itlib.Apply(sliceit.In(s), func(v int) {
		fmt.Println(v)
	})
}

```


## 🥇 Acknowledgments

The iterator interface is desgined after the stateful iterators pattern explained in the brilliant blog post from <https://ewencp.org/blog/golang-iterators/index.html>. Most functions to manipulate iterators draw inspiration from different sources such as Python and [github.com/samber/lo](https://github.com/samber/lo).


## ⚖️ License
itkit is licensed under the [Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0.txt).  

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2F0x5a17ed%2Fitkit.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2F0x5a17ed%2Fitkit?ref=badge_large)


## ☝️ Is it any good?

[yes](https://news.ycombinator.com/item?id=3067434).
