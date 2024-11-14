package tests

import (
	"context"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/Fantom-foundation/go-opera/evmcore"
	"github.com/Fantom-foundation/go-opera/gossip/gasprice"
	"github.com/Fantom-foundation/go-opera/inter"
	"github.com/Fantom-foundation/go-opera/opera"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestGasPrices_EvolutionFollowsGasPriceModel(t *testing.T) {

	net, err := StartIntegrationTestNet(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to start the fake network: %v", err)
	}
	defer net.Stop()

	// Produce a few blocks on the network.
	for range 10 {
		_, err := net.EndowAccount(common.Address{42}, 100)
		if err != nil {
			t.Fatalf("failed to endow account; %v", err)
		}
	}

	client, err := net.GetClient()
	if err != nil {
		t.Fatalf("failed to get client; %v", err)
	}
	defer client.Close()

	lastBlock, err := client.BlockByNumber(context.Background(), nil)
	if err != nil {
		t.Fatalf("failed to get block header; %v", err)
	}
	if got, minimum := lastBlock.NumberU64(), uint64(10); got < minimum {
		t.Errorf("expected at least %d blocks, got %d", minimum, got)
	}

	headers := []*types.Header{}
	for i := int64(0); i < int64(lastBlock.NumberU64()); i++ {
		header, err := client.HeaderByNumber(context.Background(), big.NewInt(i))
		if err != nil {
			t.Fatalf("failed to get block header; %v", err)
		}
		headers = append(headers, header)
	}

	if got, want := headers[0].BaseFee, gasprice.GetInitialBaseFee(); got.Cmp(want) != 0 {
		t.Fatalf("initial base fee is incorrect; got %v, want %v", got, want)
	}

	rules := opera.FakeEconomyRules()

	// Check the nano-time and duration encoded in the extra data field.
	for i := 1; i < len(headers); i++ {
		lastTime := binary.BigEndian.Uint64(headers[i-1].Extra[:8])
		currentTime := binary.BigEndian.Uint64(headers[i].Extra[:8])
		wantedDuration := currentTime - lastTime
		gotDuration := binary.BigEndian.Uint64(headers[i].Extra[8:])
		if wantedDuration != gotDuration {
			t.Errorf("duration of block %d is incorrect; got %d, want %d", i, gotDuration, wantedDuration)
		}
	}

	// Check that the gas price evolution follows the base fee pricing rules.
	for i := 1; i < len(headers); i++ {
		last := &evmcore.EvmHeader{
			BaseFee:  headers[i-1].BaseFee,
			GasLimit: headers[i-1].GasLimit,
			GasUsed:  headers[i-1].GasUsed,
			Duration: inter.Duration(binary.BigEndian.Uint64(headers[i-1].Extra[8:])),
		}
		want := gasprice.GetBaseFeeForNextBlock(last, rules)
		if got := headers[i].BaseFee; got.Cmp(want) != 0 {
			t.Errorf("base fee of block %d is incorrect; got %v, want %v", i, got, want)
		}
	}
}

func TestGasPrice_GasEvolvesAsExpectedCalculates(t *testing.T) {
	require := require.New(t)

	net, err := StartIntegrationTestNet(t.TempDir())
	require.NoError(err)
	defer net.Stop()

	// Produce a few blocks on the network.
	for range 10 {
		_, err = net.EndowAccount(common.Address{42}, 100)
		require.NoError(err)
	}

	client, err := net.GetClient()
	require.NoError(err)
	defer client.Close()

	for i := 0; i < 100; i++ {

		suggestedPrice, err := client.SuggestGasPrice(context.Background())
		require.NoError(err)

		// new block
		receipt, err := net.EndowAccount(common.Address{42}, 100)
		require.NoError(err)

		lastBlock, err := client.BlockByNumber(context.Background(), receipt.BlockNumber)
		require.NoError(err)

		diff, ok := within10Percent(suggestedPrice, lastBlock.BaseFee())
		t.Logf("i: %v, last block's base fee (%v) ok:%v, suggested price %v, diff: %v", i, lastBlock.BaseFee(), ok, suggestedPrice, diff)

	}
}

func within10Percent(a, b *big.Int) (*big.Int, bool) {
	// calculate the difference
	diff := new(big.Int).Sub(a, b)
	diff.Abs(diff)
	// calculate 10% of a
	tenPercent := new(big.Int).Mul(a, big.NewInt(10))
	tenPercent.Div(tenPercent, big.NewInt(100))
	// check if the difference is less than 10% of a
	return diff.Div(diff, a), diff.Cmp(tenPercent) < 0
}
