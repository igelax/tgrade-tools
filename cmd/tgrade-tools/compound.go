package main

import (
	"context"
	"encoding/json"
	"errors"
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

const (
	FlagMinDelegate = "min-amount"
	FlagReserve     = "reserve-amount"
)

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

			minStr, err := cmd.Flags().GetString(FlagMinDelegate)
			if err != nil {
				return err
			}
			minAmount, err := sdk.ParseCoinNormalized(minStr)
			switch {
			case err != nil:
				return err
			case minAmount.IsZero():
				return errors.New("empty minimum amount")
			case minAmount.Denom != "utgd":
				return errors.New("utgd denom required")
			}
			reserveStr, err := cmd.Flags().GetString(FlagReserve)
			if err != nil {
				return err
			}
			resAmount, err := sdk.ParseCoinNormalized(reserveStr)
			switch {
			case err != nil:
				return err
			case resAmount.Denom != "utgd":
				return errors.New("utgd denom required")
			}
			thresholdAmount := minAmount.Add(resAmount)
			claimedReward := sdk.NewCoin("utgd", sdk.ZeroInt())

			txf := tx.NewFactoryCLI(clientCtx, cmd.Flags()).
				WithTxConfig(clientCtx.TxConfig).WithAccountRetriever(clientCtx.AccountRetriever)
			queryClient := wasmtypes.NewQueryClient(clientCtx)

			var msgs []sdk.Msg
			for _, c := range []string{DistributionAddr, EngagementAddr} {
				_, contractResp, err := queryRewards(ownerAddr, queryClient, c)
				if err != nil {
					return err
				}
				if contractResp.Rewards.IsGTE(thresholdAmount) { // claim only when rewards > min
					cmd.Printf("claiming %s rewards: %d\n", c, contractResp.Rewards)
					receiverAddr := fromAddr
					msg, err := newClaimMsg(c, fromAddr, receiverAddr)
					if err != nil {
						return err
					}
					msgs = append(msgs, msg)
					claimedReward = claimedReward.Add(contractResp.Rewards)
				}
			}
			if len(msgs) == 0 {
				cmd.Print("no rewards to claim. skipping")
				return nil
			}
			if claimedReward.IsGTE(thresholdAmount) {
				delegateAmoung := claimedReward.Sub(resAmount)
				cmd.Printf("delegating claimed rewards: %d\n", delegateAmoung)
				msg := poetypes.NewMsgDelegate(ownerAddr, delegateAmoung, sdk.NewCoin("utgd", sdk.ZeroInt()))
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
	cmd.Flags().String(FlagMinDelegate, "2tgd", "The minimum amount to claim and delegate.")
	cmd.Flags().String(FlagReserve, "20000utgd", "The reward mount that should not be delegated")
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
	Rewards sdk.Coin `json:"rewards"`
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
		return nil, RewardsResponse{}, sdkerrors.Wrapf(err, "smart query: %q", query)
	}
	var contractResp RewardsResponse
	if err := json.Unmarshal(res.Data, &contractResp); err != nil {
		return nil, contractResp, sdkerrors.Wrap(err, "unmarshal result")
	}
	return res, contractResp, nil
}
