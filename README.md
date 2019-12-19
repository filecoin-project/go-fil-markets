# go-fil-components
[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://ipn.io)
[![CircleCI](https://circleci.com/gh/filecoin-project/go-fil-components.svg?style=svg)](https://circleci.com/gh/filecoin-project/go-fil-components)
[![codecov](https://codecov.io/gh/filecoin-project/go-fil-components/branch/master/graph/badge.svg)](https://codecov.io/gh/filecoin-project/go-fil-components)

This repository contains modular implementations of several Filecoin subsystems. These modules are guided by the [v1.0 and 1.1 Filecoin specification updates](https://filecoin-project.github.io/specs/#intro__changelog), which separate core blockchain consensus functionality from the storage mining, storage market, and piece data transfer subsystems. 

Separating an implementation into a blockchain component and one or more mining and market components presents an opportunity to encourage implementation diversity while re-using non-security-critical components, and also greatly ease miner-operator customisations.

## Components

* [filestore](https://github.com/filecoin-project/go-fil-components/filestore), ... 

## Contributing
PRs are welcome!  Please first read the design docs and look over the current code.  PRs against 
master require approval of at least two maintainers.  For the rest, please see our 
[CONTRIBUTING](https://github.com/filecoin-project/go-fil-components/CONTRIBUTING.md) guide.

## Project-level documentation
The filecoin-project has a [community repo](https://github.com/filecoin-project/community) that documents in more detail our policies and guidelines, such as discussion forums and chat rooms and  [Code of Conduct](https://github.com/filecoin-project/community/blob/master/CODE_OF_CONDUCT.md).

## License
This repository is dual-licensed under Apache 2.0 and MIT terms.

Copyright 2019. Protocol Labs, Inc.
