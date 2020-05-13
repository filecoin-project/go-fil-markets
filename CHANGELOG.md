# go-fil-markets changelog

# go-fil-markets 0.1.0

Initial tagged release for Filecoin Testnet Phase 2

### Contributors

❤️ Huge thank you to everyone that made this release possible! By alphabetical order, here are all the humans who contributed commits in `go-fil-markets` to date:

- [acruikshank]
- [anorth]
- [arajasek]
- [ergastic]
- [hannahhoward]
- [ingar]
- [jsign]
- [laser]
- [magik6k]
- [mishmosh]
- [shannonwells]
- [whyrusleeping]

### 🙌🏽 Want to contribute?

Would you like to contribute to this repo and don’t know how? Here are a few places you can get started:

- Check out the [Contributing Guidelines](https://github.com/filecoin-project/go-fil-markets/blob/master/CONTRIBUTING.md)
- Look for issues with the `good-first-issue` label in [go-fil-markets](https://github.com/filecoin-project/go-fil-markets/issues?utf8=%E2%9C%93&q=is%3Aissue+is%3Aopen+label%3A%22e-good-first-issue%22+)

# go-fil-markets 0.1.1

Hotfix release

# Changelog

- Upgrade spec-actors to 0.3.0

# go-fil-markets 0.1.2

Hotfix release

# Changelog

- Upgrade transitive dependencies go-ipld-prime, go-graphsync, go-data-transfer to use new, faster NodeAssembler approach in go-ipld-prime

# go-fil-markets 0.1.3

Hotfix release

# Changelog

- Upgrade transitive dependencies go-graphsync and go-data-transfer to fix a critical graphsync bug

# go-fil-markets 0.2.0

# Changelog

- See previous hotfixes which include major update of go-ipld-prime for speed
- We have seperated all calls to submit messages to chain from calls to actually
wait to see those messages on chain -- this allows us track whether we've already made the submission should the module restart
- Set Miner peer.ID on MinerDeal to fix a bug with JSON serialization
- Add an interface for listening to events on deals in the StorageClient

# go-fil-markets 0.2.1

# Changelog

- Update to data transfer 0.3.0
- Bug fixes for status maps
- Move to not keeping deal streams open in storage market

# go-fil-markets 0.2.2

# Changelog

- V26 Params update
- Revert closing streams do to incompatibilities

# go-fil-markets 0.2.3

# Changelog

- Update our network layer to hold connections open for storage deals to prevent stream resets

# go-fil-markets 0.2.4

# Changelog

- Go-Filecoin compatiblity release
- Changed data transfer request validator to operate as unified validator
- Minor bug fixes