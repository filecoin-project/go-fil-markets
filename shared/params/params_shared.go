package params

import (
	"math/big"
)

// Core network constants

// /////
// Storage

const UnixfsChunkSize uint64 = 1 << 20
const UnixfsLinksPerLevel = 1024

// /////
// Devnet settings

const TotalFilecoin = 2_000_000_000
const MiningRewardTotal = 1_400_000_000

const InitialRewardStr = "153856861913558700202"

var InitialReward *big.Int

const FilecoinPrecision = 1_000_000_000_000_000_000

// six years
// Epochs
const HalvingPeriodEpochs = 6 * 365 * 24 * 60 * 2

// TODO: Move other important consts here

func init() {
	InitialReward = new(big.Int)

	var ok bool
	InitialReward, ok = InitialReward.
		SetString(InitialRewardStr, 10)
	if !ok {
		panic("could not parse InitialRewardStr")
	}
}

// assuming 4000 messages per round, this lets us not lose any messages across a
// 10 block reorg.
const BlsSignatureCacheSize = 40000
