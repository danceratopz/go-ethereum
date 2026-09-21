// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/params/forks"
	"github.com/stretchr/testify/require"
)

func TestAmsterdamBPOSchedules(t *testing.T) {
	for _, test := range []struct {
		fork forks.Fork
		blob *BlobConfig
		set  func(*ChainConfig, *uint64, *BlobConfig)
	}{
		{forks.BPOIncrease, DefaultBPOIncreaseBlobConfig, func(c *ChainConfig, at *uint64, blob *BlobConfig) {
			c.BPOIncreaseTime, c.BlobScheduleConfig.BPOIncrease = at, blob
		}},
		{forks.BPODecrease, DefaultBPODecreaseBlobConfig, func(c *ChainConfig, at *uint64, blob *BlobConfig) {
			c.BPODecreaseTime, c.BlobScheduleConfig.BPODecrease = at, blob
		}},
	} {
		t.Run(test.fork.String(), func(t *testing.T) {
			config := *MergedTestChainConfig
			config.BPO2Time = newUint64(0)
			config.AmsterdamTime = newUint64(100)
			config.BlobScheduleConfig = &BlobScheduleConfig{
				Cancun: DefaultCancunBlobConfig,
				Prague: DefaultPragueBlobConfig,
				BPO2:   DefaultBPO2BlobConfig,
			}
			test.set(&config, newUint64(200), test.blob)
			require.NoError(t, config.CheckConfigForkOrder())
			require.Equal(t, forks.BPO2, config.LatestFork(99))
			require.Equal(t, forks.Amsterdam, config.LatestFork(199))
			require.Equal(t, DefaultBPO2BlobConfig, config.BlobConfig(config.LatestFork(199)))
			require.Equal(t, test.fork, config.LatestFork(200))
			require.Equal(t, test.blob, config.BlobConfig(config.LatestFork(200)))
			require.Equal(t, newUint64(200), config.Timestamp(test.fork))
			require.True(t, config.Rules(big.NewInt(0), true, 200).IsAmsterdam)

			// Changing an active schedule's activation time requires a rewind.
			changed := config
			test.set(&changed, newUint64(300), test.blob)
			require.Nil(t, config.CheckCompatible(&changed, 0, 199))
			err := config.CheckCompatible(&changed, 0, 200)
			require.NotNil(t, err)
			require.Equal(t, uint64(199), err.RewindToTime)

			// Each synthetic schedule requires Amsterdam and its own parameters,
			// but neither requires the other synthetic schedule.
			config.AmsterdamTime = nil
			require.ErrorContains(t, config.CheckConfigForkOrder(), "amsterdam not enabled")
			config.AmsterdamTime = newUint64(201)
			require.ErrorContains(t, config.CheckConfigForkOrder(), "unsupported fork ordering")
			config.AmsterdamTime = newUint64(100)
			test.set(&config, newUint64(200), nil)
			require.ErrorContains(t, config.CheckConfigForkOrder(), "missing entry")
		})
	}
}

func TestCheckCompatible(t *testing.T) {
	type test struct {
		stored, new   *ChainConfig
		headBlock     uint64
		headTimestamp uint64
		wantErr       *ConfigCompatError
	}
	tests := []test{
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, headBlock: 0, headTimestamp: 0, wantErr: nil},
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, headBlock: 0, headTimestamp: uint64(time.Now().Unix()), wantErr: nil},
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, headBlock: 100, wantErr: nil},
		{
			stored:    &ChainConfig{EIP150Block: big.NewInt(10)},
			new:       &ChainConfig{EIP150Block: big.NewInt(20)},
			headBlock: 9,
			wantErr:   nil,
		},
		{
			stored:    AllEthashProtocolChanges,
			new:       &ChainConfig{HomesteadBlock: nil},
			headBlock: 3,
			wantErr: &ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      nil,
				RewindToBlock: 0,
			},
		},
		{
			stored:    AllEthashProtocolChanges,
			new:       &ChainConfig{HomesteadBlock: big.NewInt(1)},
			headBlock: 3,
			wantErr: &ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      big.NewInt(1),
				RewindToBlock: 0,
			},
		},
		{
			stored:    &ChainConfig{HomesteadBlock: big.NewInt(30), EIP150Block: big.NewInt(10)},
			new:       &ChainConfig{HomesteadBlock: big.NewInt(25), EIP150Block: big.NewInt(20)},
			headBlock: 25,
			wantErr: &ConfigCompatError{
				What:          "EIP150 fork block",
				StoredBlock:   big.NewInt(10),
				NewBlock:      big.NewInt(20),
				RewindToBlock: 9,
			},
		},
		{
			stored:    &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:       &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(30)},
			headBlock: 40,
			wantErr:   nil,
		},
		{
			stored:    &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:       &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(31)},
			headBlock: 40,
			wantErr: &ConfigCompatError{
				What:          "Petersburg fork block",
				StoredBlock:   nil,
				NewBlock:      big.NewInt(31),
				RewindToBlock: 30,
			},
		},
		{
			stored:        &ChainConfig{ShanghaiTime: newUint64(10)},
			new:           &ChainConfig{ShanghaiTime: newUint64(20)},
			headTimestamp: 9,
			wantErr:       nil,
		},
		{
			stored:        &ChainConfig{ShanghaiTime: newUint64(10)},
			new:           &ChainConfig{ShanghaiTime: newUint64(20)},
			headTimestamp: 25,
			wantErr: &ConfigCompatError{
				What:         "Shanghai fork timestamp",
				StoredTime:   newUint64(10),
				NewTime:      newUint64(20),
				RewindToTime: 9,
			},
		},
	}

	for _, test := range tests {
		err := test.stored.CheckCompatible(test.new, test.headBlock, test.headTimestamp)
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("error mismatch:\nstored: %v\nnew: %v\nheadBlock: %v\nheadTimestamp: %v\nerr: %v\nwant: %v", test.stored, test.new, test.headBlock, test.headTimestamp, err, test.wantErr)
		}
	}
}

func TestConfigRules(t *testing.T) {
	c := &ChainConfig{
		LondonBlock:  new(big.Int),
		ShanghaiTime: newUint64(500),
	}
	var stamp uint64
	if r := c.Rules(big.NewInt(0), true, stamp); r.IsShanghai {
		t.Errorf("expected %v to not be shanghai", stamp)
	}
	stamp = 500
	if r := c.Rules(big.NewInt(0), true, stamp); !r.IsShanghai {
		t.Errorf("expected %v to be shanghai", stamp)
	}
	stamp = math.MaxInt64
	if r := c.Rules(big.NewInt(0), true, stamp); !r.IsShanghai {
		t.Errorf("expected %v to be shanghai", stamp)
	}
}

func TestTimestampCompatError(t *testing.T) {
	require.Equal(t, new(ConfigCompatError).Error(), "")

	errWhat := "Shanghai fork timestamp"
	require.Equal(t, newTimestampCompatError(errWhat, nil, newUint64(1681338455)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp nil, want timestamp 1681338455, rewindto timestamp 1681338454)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(1681338455), nil).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 1681338455, want timestamp nil, rewindto timestamp 1681338454)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(1681338455), newUint64(600624000)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 1681338455, want timestamp 600624000, rewindto timestamp 600623999)")

	require.Equal(t, newTimestampCompatError(errWhat, newUint64(0), newUint64(1681338455)).Error(),
		"mismatching Shanghai fork timestamp in database (have timestamp 0, want timestamp 1681338455, rewindto timestamp 0)")
}
