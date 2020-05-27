# go-fil-markets
[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://ipn.io)
[![CircleCI](https://circleci.com/gh/filecoin-project/go-fil-markets.svg?style=svg)](https://circleci.com/gh/filecoin-project/go-fil-markets)
[![codecov](https://codecov.io/gh/filecoin-project/go-fil-markets/branch/master/graph/badge.svg)](https://codecov.io/gh/filecoin-project/go-fil-markets)

This repository contains modular implementations of the [storage and retrieval market subsystems](https://filecoin-project.github.io/specs/#systems__filecoin_markets) of Filecoin. They are guided by the [v1.0 and 1.1 Filecoin specification updates](https://filecoin-project.github.io/specs/#intro__changelog). 

Separating implementations into a blockchain component and one or more mining and market components presents an opportunity to encourage implementation diversity while reusing non-security-critical components.

## Components

* **[filestore](./filestore)**: a submodule branch that is a side effect of using the ffi to
 generate commP.
* **[pieceio](./pieceio)**: utilities that take IPLD graphs and turn them into pieces.
* **[piecestore](./piecestore)**:  a database for storing deal-related PieceInfo and CIDInfo. 
* **[storagemarket](./storagemarket)**: for finding, negotiating, and consummating deals to
 store data between clients and providers (storage miners).
* **[retrievalmarket](./retrievalmarket)**: for finding, negotiating, and consummating deals to
 retrieve data between clients and providers (retrieval miners).

Related components in other repos:
* **[data transfer](https://github.com/filecoin-project/go-data-transfer)**: for exchanging piece data between clients and miners, used by storage & retrieval market modules.

### Background reading
* The [Markets in Filecoin](https://filecoin-project.github.io/specs/#systems__filecoin_markets) 
section of the Filecoin Specification contains the canonical spec
* The [Storage Market Module design doc](https://docs.google.com/document/d/1FfMUpW8vanR9FrXsybxBBbba7DzeyuCIN2uAXgE7J8U) is a more specific overview of the storage market
 component implementations
* The 
[Retrieval Market Module design doc](https://docs.google.com/document/d/1SyUDXzbGwYwoKMUWwE9_8IIjHshecLo_k7PdKQ0WK9g/edit#heading=h.uq51khvyisgr) 
is a more specific overview of the retrieval market component implementations

Install with:
`go get "github.com/filecoin-project/go-fil-markets/<MODULENAME>"`

## Usage
Documentation linked in each listed module in [Components](#Components).

## Contributing
Issues and PRs are welcome! Please first read the [background reading](#background-reading) and [CONTRIBUTING](.go-fil-markets/CONTRIBUTING.md) guide, and look over the current code. PRs against master require approval of at least two maintainers. 

Day-to-day discussion takes place in the #fil-components channel of the [Filecoin project chat](https://github.com/filecoin-project/community#chat). Usage or design questions are welcome.

## Project-level documentation
The filecoin-project has a [community repo](https://github.com/filecoin-project/community) with more detail about our resources and policies, such as the [Code of Conduct](https://github.com/filecoin-project/community/blob/master/CODE_OF_CONDUCT.md).

## License
This repository is dual-licensed under Apache 2.0 and MIT terms.

Copyright 2019. Protocol Labs, Inc.
