package main

import (
	"os"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/cobra"

	"github.com/confio/tgrade/app"
	appparams "github.com/confio/tgrade/app/params"
)

const (
	// DistributionAddr is the address of the distribution contract. It can be queried via `tgrade q poe contract-address DISTRIBUTION`
	DistributionAddr = "tgrade1wl59k23zngj34l7d42y9yltask7rjlnxgccawc7ltrknp6n52fps2p2ent"
	// EngagementAddr is the address of the engagement contract. It can be queried via `tgrade q poe contract-address ENGAGEMENT`
	EngagementAddr = "tgrade14hj2tavq8fpesdwxxcu44rty3hh90vhujrvcmstl4zr3txmfvw9s07fvfr"
)

func NewRootCmd() (*cobra.Command, appparams.EncodingConfig) {
	encodingConfig := app.MakeEncodingConfig()

	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(app.Bech32PrefixAccAddr, app.Bech32PrefixAccPub)
	cfg.SetBech32PrefixForValidator(app.Bech32PrefixValAddr, app.Bech32PrefixValPub)
	cfg.SetBech32PrefixForConsensusNode(app.Bech32PrefixConsAddr, app.Bech32PrefixConsPub)
	cfg.SetAddressVerifier(wasmtypes.VerifyAddressLen())
	cfg.Seal()

	initClientCtx := client.Context{}.
		WithCodec(encodingConfig.Codec).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithInput(os.Stdin).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(flags.BroadcastBlock).
		WithHomeDir(app.DefaultNodeHome).
		WithViper("")

	rootCmd := &cobra.Command{
		Use:   "tgrade-tools",
		Short: "Helpful tgrade CLI tools and queries",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// set the default command outputs
			cmd.SetOut(cmd.OutOrStdout())
			cmd.SetErr(cmd.ErrOrStderr())

			initClientCtx, err := client.ReadPersistentCommandFlags(initClientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			initClientCtx, err = config.ReadFromClientConfig(initClientCtx)
			if err != nil {
				return err
			}

			if err := client.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
				return err
			}

			return server.InterceptConfigsPreRunHandler(cmd, "", nil)
		},
	}

	rootCmd.AddCommand(ExecuteCompound())
	rootCmd.AddCommand(StartPrometheusCmd())
	rootCmd.AddCommand(GetQueryCmd())
	rootCmd.PersistentFlags().String(flags.FlagChainID, "", "The network chain ID")
	return rootCmd, encodingConfig
}
