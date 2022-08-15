package main

import (
	"context"
	"encoding/json"
	"fmt"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/spf13/cobra"

	poetypes "github.com/confio/tgrade/x/poe/types"
)

type Coin struct {
	Denom  string `json:"denom"`
	Amount uint64 `json:"amount,string"`
}

func ExecuteCompound() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compound",
		Short: "Query, claim and stake all rewards. Watch your balance to not run out of tokens to pay fees",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			fromAddr := clientCtx.GetFromAddress().String()
			ownerAddr, err := sdk.AccAddressFromBech32(fromAddr)
			if err != nil {
				return sdkerrors.Wrap(err, "address")
			}
			var minAmount uint64 = 2_000_000

			txf := tx.NewFactoryCLI(clientCtx, cmd.Flags()).
				WithTxConfig(clientCtx.TxConfig).WithAccountRetriever(clientCtx.AccountRetriever)

			queryClient := wasmtypes.NewQueryClient(clientCtx)
			var msgs []sdk.Msg
			var claimedReward uint64
			for _, c := range []string{DistributionAddr, EngagementAddr} {
				_, contractResp, err := queryRewards(ownerAddr, queryClient, c)
				if err != nil {
					return err
				}
				if contractResp.Rewards.Amount > minAmount { // claim only when rewards > min
					cmd.Printf("claiming %s rewards: %d\n", c, contractResp.Rewards.Amount)
					receiverAddr := fromAddr
					msg, err := newClaimMsg(c, fromAddr, receiverAddr)
					if err != nil {
						return err
					}
					msgs = append(msgs, msg)
					claimedReward += contractResp.Rewards.Amount
				}
			}
			if len(msgs) == 0 {
				cmd.Print("no rewards to claim. skipping")
				return nil
			}
			if claimedReward > minAmount {
				cmd.Printf("delegating claimed rewards: %d\n", claimedReward)
				msg := poetypes.NewMsgDelegate(ownerAddr, sdk.NewInt64Coin("utgd", int64(claimedReward)), sdk.NewInt64Coin("utgd", 0))
				if err := msg.ValidateBasic(); err != nil {
					return sdkerrors.Wrap(err, "delegate msg")
				}
				msgs = append(msgs, msg)
			}

			if err := tx.GenerateOrBroadcastTxWithFactory(clientCtx, txf, msgs...); err != nil {
				return err
			}
			return nil
		},
	}
	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

func newClaimMsg(contractAddr string, fromAddr string, receiverAddr string) (*wasmtypes.MsgExecuteContract, error) {
	msg := &wasmtypes.MsgExecuteContract{
		Sender:   fromAddr,
		Contract: contractAddr,
		Msg:      []byte(fmt.Sprintf(`{"withdraw_rewards": {"owner": %q, "receiver": %q}}`, fromAddr, receiverAddr)), // note: owner+receiver not supported for trusted circles
	}
	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}
	return msg, nil
}

// RewardsResponse smart query response
type RewardsResponse struct {
	Rewards Coin `json:"rewards"`
}

func queryRewards(ownerAddr sdk.AccAddress, queryClient wasmtypes.QueryClient, contractAddr string) (*wasmtypes.QuerySmartContractStateResponse, RewardsResponse, error) {
	query := fmt.Sprintf(`{"withdrawable_rewards": { "owner": %q}}`, ownerAddr.String())
	res, err := queryClient.SmartContractState(
		context.Background(),
		&wasmtypes.QuerySmartContractStateRequest{
			Address:   contractAddr,
			QueryData: []byte(query),
		},
	)
	if err != nil {
		return nil, struct {
			Rewards Coin `json:"rewards"`
		}{}, sdkerrors.Wrapf(err, "smart query: %q", query)
	}
	contractResp := struct {
		Rewards Coin `json:"rewards"`
	}{}
	if err := json.Unmarshal(res.Data, &contractResp); err != nil {
		return nil, struct {
			Rewards Coin `json:"rewards"`
		}{}, sdkerrors.Wrap(err, "unmarshal result")
	}
	return res, contractResp, nil
}
