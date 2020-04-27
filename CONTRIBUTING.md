# Contributing to this repo

First, thank you for your interest in contributing to this project! Before you pick up your first issue and start
changing code, please:

1. Review all documentation for the module you're interested in.
1. Look through the [issues for this repo](https://github.com/filecoin-project/go-fil-markets/issues) for relevant discussions.
1. If you have questions about an issue, post a comment in the issue.
1. If you want to submit changes that aren't covered by an issue, file a new one with your proposal, outlining what problem you found/feature you want to implement, and how you intend to implement a solution.

For best results, before submitting a PR, make sure:
1. It has met all acceptance criteria for the issue.
1. It addresses only the one issue and does not make other, irrelevant changes.
1. Your code conforms to our coding style guide.
1. You have adequate test coverage (this should be indicated by CI results anyway).
1. If you like, check out [current PRs](https://github.com/filecoin-project/go-fil-markets/pulls) to see how others do it.

Special Note:
If editing README.md, please conform to the [standard readme specification](https://github.com/RichardLitt/standard-readme/blob/master/spec.md).

### PR Process

Active development of `go-fil-markets` occurs on the `development` branch. All PRs should be made to the `development` branch, which is the default branch on Github.

Before a PR can be merged to `development`, it must:
1. Pass continuous integration.
1. Be rebased and up to date with the development branch
1. Be approved by at least two maintainers

When merging normal PRs to development, always use squash and merge to maintain a linear commit history.

### Release Process

The `master` branch is consider our production branch, and should always be up to date with the latest tagged release.

There are two ways to update `master`. The first is to cut a new full release by making a PR from `development` to `master` with the latest changes from development. When the PR is merged, it MUST be merged via a merge commit. At this point, make a new tagged version release and seperately, make a second PR back to `development` from `master` once the release is verified as ready to go, to get the merge commit in development. (maintaining a shared commit history). Only a lead maintainer may merge to `master`.

The second way to update `master` is to hotfix it. Hot fixes are branched off `master` and merged directly back into `master` to fix critical bugs in a production release. When a creating a hotfix, create a PR to `master` and once approved, merge with `master` then create a seperate PR to merge `master` back to `development` with the hotfix to maintain the shared commit history

Following the release of Filecoin Mainnet, this library will following a semantic versioning scheme for tagged releases.

### Testing

- All new code should be accompanied by unit tests. Prefer focused unit tests to integration tests for thorough validation of behaviour. Existing code is not necessarily a good model, here.
- Integration tests should test integration, not comprehensive functionality
- Tests should be placed in a separate package named `$PACKAGE_test`. For example, a test of the `chain` package should live in a package named `chain_test`. In limited situations, exceptions may be made for some "white box" tests placed in the same package as the code it tests.

### Conventions and Style

#### Imports
We use the following import ordering.
```
import (
        [stdlib packages, alpha-sorted]
        <single, empty line>
        [external packages]
        <single, empty line>
        [go-fil-markets packages]
)
```

Where a package name does not match its directory name, an explicit alias is expected (`goimports` will add this for you).

Example:

```go
import (
	"context"
	"testing"

	cmds "github.com/ipfs/go-ipfs-cmds"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-fil-markets/filestore/file"
)
```

#### Comments
Comments are a communication to other developers (including your future self) to help them understand and maintain code. Good comments describe the _intent_ of the code, without repeating the procedures directly.

- A `TODO:` comment describes a change that is desired but could not be immediately implemented. It must include a reference to a GitHub issue outlining whatever prevents the thing being done now (which could just be a matter of priority).
- A `NOTE:` comment indicates an aside, some background info, or ideas for future improvement, rather than the intent of the current code. It's often fine to document such ideas alongside the code rather than an issue (at the loss of a space for discussion).
- `FIXME`, `HACK`, `XXX` and similar tags indicating that some code is to be avoided in favour of `TODO`, `NOTE` or some straight prose.
